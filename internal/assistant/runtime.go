package assistant

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/pkg/tarsclient"
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
	transcript, err := deps.Transcriber.Transcribe(ctx, strings.TrimSpace(audioPath))
	if err != nil {
		return VoiceTurnResult{}, err
	}
	return executeChatTurn(ctx, deps, transcript)
}

type StartOptions struct {
	ServerURL       string
	SessionID       string
	APIToken        string
	WorkspaceDir    string
	Hotkey          string
	AudioInput      string
	WhisperBin      string
	WhisperModel    string
	WhisperLanguage string
	FFmpegBin       string
	TTSBin          string
	Stdin           io.Reader
	Stdout          io.Writer
	Stderr          io.Writer
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
	whisperBin := defaultIfEmpty(opts.WhisperBin, "whisper-cli")
	deps := VoiceTurnDeps{
		Transcriber: commandTranscriber{
			binary:    whisperBin,
			modelPath: defaultWhisperModelPath(opts.WhisperModel, whisperBin),
			language:  defaultIfEmpty(opts.WhisperLanguage, "ko"),
		},
		ChatClient: chatClient,
		Speaker:    commandSpeaker{binary: defaultIfEmpty(opts.TTSBin, "say")},
		SessionID:  strings.TrimSpace(opts.SessionID),
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
		fmt.Fprintln(stdout, "press hotkey to open assistant popup, Ctrl+C to exit")
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
		if stopErr := treatRecordingStopError(rec.stop(), wavPath); stopErr != nil {
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
	popup, err := newPopupPresenter()
	if err != nil {
		fmt.Fprintf(stderr, "assistant warning: popup unavailable, fallback to hold-to-talk: %v\n", err)
		return runHotkeyVoiceMode(ctx, listener, deps, voiceDir, ffmpegBin, audioInput, stdout, stderr)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := listener.WaitPress(ctx); err != nil {
			return err
		}
		if err := listener.WaitRelease(ctx); err != nil {
			return err
		}
		result, err := popup.Prompt(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "assistant warning: failed to open popup: %v\n", err)
			continue
		}
		switch result.Action {
		case popupActionCancel:
			continue
		case popupActionSend:
			turnResult, runErr := RunTextTurn(ctx, deps, result.Text)
			if runErr != nil {
				fmt.Fprintf(stderr, "assistant warning: %v\n", runErr)
				_ = popup.ShowError(ctx, runErr.Error())
				continue
			}
			if writeErr := writeTurnResult(&deps, turnResult, stdout, stderr); writeErr != nil {
				fmt.Fprintf(stderr, "assistant warning: %v\n", writeErr)
				_ = popup.ShowError(ctx, writeErr.Error())
				continue
			}
			_ = popup.ShowResult(ctx, turnResult)
		case popupActionMic:
			if runErr := handlePopupVoiceTurn(ctx, popup, &deps, voiceDir, ffmpegBin, audioInput, stdout, stderr); runErr != nil {
				fmt.Fprintf(stderr, "assistant warning: %v\n", runErr)
				_ = popup.ShowError(ctx, runErr.Error())
			}
		}
	}
}

func runHotkeyVoiceMode(
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
		if stopErr := treatRecordingStopError(rec.stop(), wavPath); stopErr != nil {
			fmt.Fprintf(stderr, "assistant warning: failed to stop recorder: %v\n", stopErr)
			continue
		}
		if runErr := handleVoiceTurn(ctx, &deps, wavPath, stdout, stderr); runErr != nil {
			fmt.Fprintf(stderr, "assistant warning: %v\n", runErr)
		}
	}
}

func handleTextTurn(ctx context.Context, deps *VoiceTurnDeps, text string, stdout io.Writer, stderr io.Writer) error {
	if deps == nil {
		return fmt.Errorf("voice dependencies are required")
	}
	result, err := RunTextTurn(ctx, *deps, text)
	if err != nil {
		return err
	}
	return writeTurnResult(deps, result, stdout, stderr)
}

func handleVoiceTurn(ctx context.Context, deps *VoiceTurnDeps, wavPath string, stdout io.Writer, stderr io.Writer) error {
	if deps == nil {
		return fmt.Errorf("voice dependencies are required")
	}
	result, err := RunVoiceTurn(ctx, *deps, wavPath)
	if err != nil {
		return err
	}
	return writeTurnResult(deps, result, stdout, stderr)
}

func writeTurnResult(deps *VoiceTurnDeps, result VoiceTurnResult, stdout io.Writer, stderr io.Writer) error {
	if deps == nil {
		return fmt.Errorf("voice dependencies are required")
	}
	deps.SessionID = result.SessionID
	fmt.Fprintf(stdout, "you> %s\n", result.Transcript)
	fmt.Fprintf(stdout, "tars> %s\n", result.AssistantReply)
	if strings.TrimSpace(result.TTSError) != "" {
		fmt.Fprintf(stderr, "assistant warning: tts fallback to text (%s)\n", result.TTSError)
	}
	return nil
}

func handlePopupVoiceTurn(
	ctx context.Context,
	popup popupPresenter,
	deps *VoiceTurnDeps,
	voiceDir string,
	ffmpegBin string,
	audioInput string,
	stdout io.Writer,
	stderr io.Writer,
) error {
	if popup == nil {
		return fmt.Errorf("popup presenter is required")
	}
	wavPath := filepath.Join(voiceDir, time.Now().UTC().Format("20060102-150405")+".wav")
	rec, err := startRecording(ctx, ffmpegBin, audioInput, wavPath)
	if err != nil {
		return fmt.Errorf("failed to start recorder: %w", err)
	}
	send, waitErr := popup.WaitRecordingStop(ctx)
	stopErr := treatRecordingStopError(rec.stop(), wavPath)
	if waitErr != nil {
		if stopErr != nil {
			return fmt.Errorf("failed to stop recorder after popup error: %v (popup error: %w)", stopErr, waitErr)
		}
		return waitErr
	}
	if stopErr != nil {
		return fmt.Errorf("failed to stop recorder: %w", stopErr)
	}
	if !send {
		return nil
	}
	if deps == nil {
		return fmt.Errorf("voice dependencies are required")
	}
	result, err := RunVoiceTurn(ctx, *deps, wavPath)
	if err != nil {
		return err
	}
	if err := writeTurnResult(deps, result, stdout, stderr); err != nil {
		return err
	}
	return popup.ShowResult(ctx, result)
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
	binary    string
	modelPath string
	language  string
}

func (c commandTranscriber) Transcribe(ctx context.Context, audioPath string) (string, error) {
	cmd := exec.CommandContext(ctx, strings.TrimSpace(c.binary), buildWhisperArgs(audioPath, c.modelPath, c.language)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		text := strings.TrimSpace(stderr.String())
		if strings.Contains(strings.ToLower(text), "failed to open") && strings.TrimSpace(c.modelPath) == "" {
			return "", fmt.Errorf("transcribe failed: %w: %s (hint: set --whisper-model or TARS_ASSISTANT_WHISPER_MODEL to a real ggml model path)", err, text)
		}
		if text == "" {
			text = strings.TrimSpace(stdout.String())
		}
		return "", fmt.Errorf("transcribe failed: %w: %s", err, text)
	}
	text := extractTranscriptOutput(stdout.String())
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

func buildWhisperArgs(audioPath string, modelPath string, language string) []string {
	args := make([]string, 0, 7)
	if strings.TrimSpace(modelPath) != "" {
		args = append(args, "-m", strings.TrimSpace(modelPath))
	}
	args = append(args, "-np", "-nt")
	if strings.TrimSpace(language) != "" {
		args = append(args, "-l", strings.TrimSpace(language))
	}
	return append(args, strings.TrimSpace(audioPath))
}

func extractTranscriptOutput(raw string) string {
	lines := strings.Split(raw, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "whisper_") || strings.HasPrefix(lower, "ggml_") || strings.HasPrefix(lower, "system_info:") || strings.HasPrefix(lower, "main: processing") {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return strings.TrimSpace(strings.Join(filtered, " "))
}

func defaultWhisperModelPath(raw string, binary string) string {
	if strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	dirs := []string{
		filepath.Join(filepath.Dir(strings.TrimSpace(binary)), "..", "share", "whisper-cpp"),
		"/opt/homebrew/opt/whisper-cpp/share/whisper-cpp",
		"/opt/homebrew/share/whisper-cpp",
		"./models",
		"../models",
	}
	models := []string{
		"ggml-base.bin",
		"ggml-base.en.bin",
		"ggml-small.bin",
		"ggml-small.en.bin",
		"ggml-medium.bin",
		"ggml-medium.en.bin",
		"ggml-large-v3.bin",
	}
	for _, dir := range dirs {
		for _, model := range models {
			candidate := filepath.Clean(filepath.Join(dir, model))
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}

func treatRecordingStopError(stopErr error, wavPath string) error {
	if stopErr == nil {
		return nil
	}
	lower := strings.ToLower(stopErr.Error())
	if strings.Contains(lower, "signal") {
		return nil
	}
	if strings.Contains(lower, "exit status 255") && wavLooksUsable(wavPath) {
		return nil
	}
	return stopErr
}

func wavLooksUsable(wavPath string) bool {
	info, err := os.Stat(strings.TrimSpace(wavPath))
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() >= 1024
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
