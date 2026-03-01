package assistant

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/pkg/tarsclient"
)

const (
	DefaultHotkey     = "Ctrl+Option+Space"
	DefaultAudioInput = "default"
)

type Transcriber interface {
	Transcribe(ctx context.Context, audioPath string) (string, error)
}

type VoiceChatClient interface {
	Chat(ctx context.Context, message string, sessionID string) (assistantReply string, nextSessionID string, err error)
}

type Speaker interface {
	Speak(ctx context.Context, text string) error
}

type VoiceTurnDeps struct {
	Transcriber Transcriber
	ChatClient  VoiceChatClient
	Speaker     Speaker
	SessionID   string
}

type VoiceTurnResult struct {
	Transcript     string
	AssistantReply string
	SessionID      string
	TTSError       string
}

func RunVoiceTurn(ctx context.Context, deps VoiceTurnDeps, audioPath string) (VoiceTurnResult, error) {
	if deps.Transcriber == nil {
		return VoiceTurnResult{}, fmt.Errorf("transcriber is required")
	}
	if deps.ChatClient == nil {
		return VoiceTurnResult{}, fmt.Errorf("chat client is required")
	}
	transcript, err := deps.Transcriber.Transcribe(ctx, strings.TrimSpace(audioPath))
	if err != nil {
		return VoiceTurnResult{}, err
	}
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return VoiceTurnResult{}, fmt.Errorf("empty transcript")
	}
	reply, nextSession, err := deps.ChatClient.Chat(ctx, transcript, strings.TrimSpace(deps.SessionID))
	if err != nil {
		return VoiceTurnResult{}, err
	}
	result := VoiceTurnResult{
		Transcript:     transcript,
		AssistantReply: strings.TrimSpace(reply),
		SessionID:      strings.TrimSpace(nextSession),
	}
	if result.SessionID == "" {
		result.SessionID = strings.TrimSpace(deps.SessionID)
	}
	if deps.Speaker != nil && strings.TrimSpace(result.AssistantReply) != "" {
		if err := deps.Speaker.Speak(ctx, result.AssistantReply); err != nil {
			result.TTSError = err.Error()
		}
	}
	return result, nil
}

type StartOptions struct {
	ServerURL    string
	SessionID    string
	APIToken     string
	WorkspaceDir string
	Hotkey       string
	AudioInput   string
	WhisperBin   string
	FFmpegBin    string
	TTSBin       string
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
}

func Start(ctx context.Context, opts StartOptions) error {
	stdin := opts.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	workspaceDir := strings.TrimSpace(opts.WorkspaceDir)
	if workspaceDir == "" {
		workspaceDir = "./workspace"
	}
	voiceDir := filepath.Join(workspaceDir, "_shared", "voice", "inbox")
	if err := os.MkdirAll(voiceDir, 0o755); err != nil {
		return fmt.Errorf("create voice inbox: %w", err)
	}

	chatClient := apiChatClient{client: tarsclient.New(tarsclient.Config{
		ServerURL: strings.TrimSpace(opts.ServerURL),
		APIToken:  strings.TrimSpace(opts.APIToken),
	})}
	deps := VoiceTurnDeps{
		Transcriber: commandTranscriber{binary: defaultIfEmpty(opts.WhisperBin, "whisper-cli")},
		ChatClient:  chatClient,
		Speaker:     commandSpeaker{binary: defaultIfEmpty(opts.TTSBin, "say")},
		SessionID:   strings.TrimSpace(opts.SessionID),
	}

	hotkey := strings.TrimSpace(opts.Hotkey)
	if hotkey == "" {
		hotkey = DefaultHotkey
	}
	audioInput := resolveAVFoundationAudioInput(opts.AudioInput)
	listener, listenerErr := tryCreateHotkeyListener(hotkey)
	mode, warning := resolveRuntimeMode(listener, hotkey, listenerErr)
	switch mode {
	case runtimeModeHotkey:
		fmt.Fprintf(stdout, "assistant started (global hotkey=%s)\n", hotkey)
		fmt.Fprintln(stdout, "hold hotkey to record, release to send, Ctrl+C to exit")
		if warning != "" {
			fmt.Fprintf(stderr, "assistant warning: %s\n", warning)
		}
		defer listener.Close()
		return runHotkeyMode(ctx, listener, deps, voiceDir, defaultIfEmpty(opts.FFmpegBin, "ffmpeg"), audioInput, stdout, stderr)
	default:
		fmt.Fprintf(stdout, "assistant started (hotkey=%s, fallback=enter)\n", hotkey)
		if warning != "" {
			fmt.Fprintf(stderr, "assistant warning: %s\n", warning)
		}
		fmt.Fprintln(stdout, "press ENTER to start recording, ENTER again to stop, type /quit to exit")
		return runFallbackInputMode(ctx, stdin, deps, voiceDir, defaultIfEmpty(opts.FFmpegBin, "ffmpeg"), audioInput, stdout, stderr)
	}
}

func tryCreateHotkeyListener(raw string) (hotkeyListener, string) {
	listener, err := newGlobalHotkeyListener(raw)
	if err != nil {
		return nil, err.Error()
	}
	return listener, ""
}

func runFallbackInputMode(
	ctx context.Context,
	stdin io.Reader,
	deps VoiceTurnDeps,
	voiceDir string,
	ffmpegBin string,
	audioInput string,
	stdout io.Writer,
	stderr io.Writer,
) error {
	reader := bufio.NewReader(stdin)
	state := PushToTalkState{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Fprint(stdout, "assistant> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if strings.EqualFold(strings.TrimSpace(line), "/quit") {
			return nil
		}
		if !state.HandlePressed() {
			continue
		}
		wavPath := filepath.Join(voiceDir, time.Now().UTC().Format("20060102-150405")+".wav")
		rec, err := startRecording(ctx, ffmpegBin, audioInput, wavPath)
		if err != nil {
			state.HandleReleased()
			fmt.Fprintf(stderr, "assistant warning: failed to start recorder: %v\n", err)
			continue
		}
		fmt.Fprintln(stdout, "recording... press ENTER to stop")
		_, _ = reader.ReadString('\n')
		state.HandleReleased()
		if stopErr := rec.stop(); stopErr != nil {
			fmt.Fprintf(stderr, "assistant warning: failed to stop recorder: %v\n", stopErr)
			continue
		}
		if runErr := handleVoiceTurn(ctx, &deps, wavPath, stdout, stderr); runErr != nil {
			fmt.Fprintf(stderr, "assistant warning: %v\n", runErr)
		}
	}
}

func runHotkeyMode(
	ctx context.Context,
	listener hotkeyListener,
	deps VoiceTurnDeps,
	voiceDir string,
	ffmpegBin string,
	audioInput string,
	stdout io.Writer,
	stderr io.Writer,
) error {
	state := PushToTalkState{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := listener.WaitPress(ctx); err != nil {
			return err
		}
		if !state.HandlePressed() {
			continue
		}
		wavPath := filepath.Join(voiceDir, time.Now().UTC().Format("20060102-150405")+".wav")
		rec, err := startRecording(ctx, ffmpegBin, audioInput, wavPath)
		if err != nil {
			fmt.Fprintf(stderr, "assistant warning: failed to start recorder: %v\n", err)
			state.HandleReleased()
			if waitErr := listener.WaitRelease(ctx); waitErr != nil {
				return waitErr
			}
			continue
		}
		fmt.Fprintln(stdout, "recording... release hotkey to stop")
		if err := listener.WaitRelease(ctx); err != nil {
			_ = rec.stop()
			return err
		}
		state.HandleReleased()
		if stopErr := rec.stop(); stopErr != nil {
			fmt.Fprintf(stderr, "assistant warning: failed to stop recorder: %v\n", stopErr)
			continue
		}
		if runErr := handleVoiceTurn(ctx, &deps, wavPath, stdout, stderr); runErr != nil {
			fmt.Fprintf(stderr, "assistant warning: %v\n", runErr)
		}
	}
}

func handleVoiceTurn(ctx context.Context, deps *VoiceTurnDeps, wavPath string, stdout io.Writer, stderr io.Writer) error {
	if deps == nil {
		return fmt.Errorf("voice dependencies are required")
	}
	result, err := RunVoiceTurn(ctx, *deps, wavPath)
	if err != nil {
		return err
	}
	deps.SessionID = result.SessionID
	fmt.Fprintf(stdout, "you> %s\n", result.Transcript)
	fmt.Fprintf(stdout, "tars> %s\n", result.AssistantReply)
	if strings.TrimSpace(result.TTSError) != "" {
		fmt.Fprintf(stderr, "assistant warning: tts fallback to text (%s)\n", result.TTSError)
	}
	return nil
}

type apiChatClient struct {
	client *tarsclient.Client
}

func (a apiChatClient) Chat(ctx context.Context, message string, sessionID string) (string, string, error) {
	if a.client == nil {
		return "", "", fmt.Errorf("chat client is nil")
	}
	res, err := a.client.StreamChat(ctx, tarsclient.ChatRequest{
		Message:   strings.TrimSpace(message),
		SessionID: strings.TrimSpace(sessionID),
	}, nil, nil)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(res.Assistant), strings.TrimSpace(res.SessionID), nil
}

type commandTranscriber struct {
	binary string
}

func (c commandTranscriber) Transcribe(ctx context.Context, audioPath string) (string, error) {
	cmd := exec.CommandContext(ctx, strings.TrimSpace(c.binary), strings.TrimSpace(audioPath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("transcribe failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "", fmt.Errorf("transcribe failed: empty output")
	}
	return text, nil
}

type commandSpeaker struct {
	binary string
}

func (c commandSpeaker) Speak(ctx context.Context, text string) error {
	cmd := exec.CommandContext(ctx, strings.TrimSpace(c.binary), strings.TrimSpace(text))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("speak failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

type ffmpegRecording struct {
	cmd *exec.Cmd
}

func startRecording(ctx context.Context, ffmpegBin string, audioInput string, wavPath string) (*ffmpegRecording, error) {
	args := []string{"-y", "-f", "avfoundation", "-i", resolveAVFoundationAudioInput(audioInput), "-ac", "1", "-ar", "16000", "-acodec", "pcm_s16le", strings.TrimSpace(wavPath)}
	cmd := exec.CommandContext(ctx, strings.TrimSpace(ffmpegBin), args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &ffmpegRecording{cmd: cmd}, nil
}

func (r *ffmpegRecording) stop() error {
	if r == nil || r.cmd == nil || r.cmd.Process == nil {
		return nil
	}
	_ = r.cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- r.cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return nil
		}
		if strings.Contains(strings.ToLower(err.Error()), "signal") {
			return nil
		}
		return err
	case <-time.After(3 * time.Second):
		_ = r.cmd.Process.Kill()
		return <-done
	}
}

func defaultIfEmpty(value, fallback string) string {
	v := strings.TrimSpace(value)
	if v != "" {
		return v
	}
	return strings.TrimSpace(fallback)
}

func resolveAVFoundationAudioInput(raw string) string {
	input := strings.TrimSpace(raw)
	if input == "" {
		return ":" + DefaultAudioInput
	}
	if strings.HasPrefix(input, ":") {
		return input
	}
	if strings.Contains(input, ":") {
		return input
	}
	return ":" + input
}
