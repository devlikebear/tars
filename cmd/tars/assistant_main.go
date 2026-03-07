package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/devlikebear/tarsncase/internal/assistant"
	protocol "github.com/devlikebear/tarsncase/pkg/tarsclient"
	"github.com/spf13/cobra"
)

type assistantOptions struct {
	action      string
	serverURL   string
	sessionID   string
	apiToken    string
	workspace   string
	hotkey      string
	audioInput  string
	whisperBin  string
	whisperModel string
	ffmpegBin   string
	ttsBin      string
	label       string
	plistPath   string
	stdoutLog   string
	stderrLog   string
	keepAlive   bool
	runAtLoad   bool
	installLoad bool
}

var assistantRunner = runAssistantCommand

func defaultAssistantOptions() assistantOptions {
	serverURL := strings.TrimSpace(os.Getenv("TARS_SERVER_URL"))
	if serverURL == "" {
		serverURL = protocol.DefaultServerURL
	}
	return assistantOptions{
		serverURL: serverURL,
		sessionID: strings.TrimSpace(os.Getenv("TARS_SESSION_ID")),
		apiToken:  strings.TrimSpace(os.Getenv("TARS_API_TOKEN")),
		workspace: strings.TrimSpace(firstNonEmpty(os.Getenv("TARS_WORKSPACE_DIR"), "./workspace")),
		hotkey:    assistant.DefaultHotkey,
		audioInput: strings.TrimSpace(firstNonEmpty(
			os.Getenv("TARS_ASSISTANT_AUDIO_INPUT"),
			assistant.DefaultAudioInput,
		)),
		whisperBin: strings.TrimSpace(firstNonEmpty(
			os.Getenv("TARS_ASSISTANT_WHISPER_BIN"),
			"whisper-cli",
		)),
		whisperModel: strings.TrimSpace(os.Getenv("TARS_ASSISTANT_WHISPER_MODEL")),
		ffmpegBin: strings.TrimSpace(firstNonEmpty(
			os.Getenv("TARS_ASSISTANT_FFMPEG_BIN"),
			"ffmpeg",
		)),
		ttsBin: strings.TrimSpace(firstNonEmpty(
			os.Getenv("TARS_ASSISTANT_TTS_BIN"),
			"say",
		)),
		label:       assistant.DefaultLaunchAgentLabel,
		keepAlive:   true,
		runAtLoad:   true,
		installLoad: true,
	}
}

func newAssistantCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := defaultAssistantOptions()
	cmd := &cobra.Command{
		Use:   "assistant",
		Short: "Run local assistant voice/runtime helpers",
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start assistant runtime (push-to-talk fallback mode)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			startOpts := opts
			startOpts.action = "start"
			return assistantRunner(cmd.Context(), startOpts, stdout, stderr)
		},
	}
	startCmd.Flags().StringVar(&opts.serverURL, "server-url", opts.serverURL, "tars server url")
	startCmd.Flags().StringVar(&opts.sessionID, "session", opts.sessionID, "session id")
	startCmd.Flags().StringVar(&opts.apiToken, "api-token", opts.apiToken, "api token")
	startCmd.Flags().StringVar(&opts.workspace, "workspace-dir", opts.workspace, "workspace directory")
	startCmd.Flags().StringVar(&opts.hotkey, "hotkey", opts.hotkey, "global hotkey (display only in fallback mode)")
	startCmd.Flags().StringVar(&opts.audioInput, "audio-input", opts.audioInput, "avfoundation audio input (default|index|name)")
	startCmd.Flags().StringVar(&opts.whisperBin, "whisper-bin", opts.whisperBin, "speech-to-text command")
	startCmd.Flags().StringVar(&opts.whisperModel, "whisper-model", opts.whisperModel, "whisper model path")
	startCmd.Flags().StringVar(&opts.ffmpegBin, "ffmpeg-bin", opts.ffmpegBin, "ffmpeg command path")
	startCmd.Flags().StringVar(&opts.ttsBin, "tts-bin", opts.ttsBin, "text-to-speech command")

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check assistant runtime dependencies",
		RunE: func(cmd *cobra.Command, _ []string) error {
			doctorOpts := opts
			doctorOpts.action = "doctor"
			return assistantRunner(cmd.Context(), doctorOpts, stdout, stderr)
		},
	}
	doctorCmd.Flags().StringVar(&opts.whisperBin, "whisper-bin", opts.whisperBin, "speech-to-text command")
	doctorCmd.Flags().StringVar(&opts.whisperModel, "whisper-model", opts.whisperModel, "whisper model path")
	doctorCmd.Flags().StringVar(&opts.ffmpegBin, "ffmpeg-bin", opts.ffmpegBin, "ffmpeg command path")
	doctorCmd.Flags().StringVar(&opts.ttsBin, "tts-bin", opts.ttsBin, "text-to-speech command")

	installCmd := &cobra.Command{
		Use:   "install-launchagent",
		Short: "Install LaunchAgent plist for assistant autostart (macOS)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			installOpts := opts
			installOpts.action = "install-launchagent"
			return assistantRunner(cmd.Context(), installOpts, stdout, stderr)
		},
	}
	installCmd.Flags().StringVar(&opts.serverURL, "server-url", opts.serverURL, "tars server url")
	installCmd.Flags().StringVar(&opts.sessionID, "session", opts.sessionID, "session id")
	installCmd.Flags().StringVar(&opts.apiToken, "api-token", opts.apiToken, "api token")
	installCmd.Flags().StringVar(&opts.workspace, "workspace-dir", opts.workspace, "workspace directory")
	installCmd.Flags().StringVar(&opts.hotkey, "hotkey", opts.hotkey, "global hotkey")
	installCmd.Flags().StringVar(&opts.audioInput, "audio-input", opts.audioInput, "avfoundation audio input (default|index|name)")
	installCmd.Flags().StringVar(&opts.whisperBin, "whisper-bin", opts.whisperBin, "speech-to-text command")
	installCmd.Flags().StringVar(&opts.whisperModel, "whisper-model", opts.whisperModel, "whisper model path")
	installCmd.Flags().StringVar(&opts.ffmpegBin, "ffmpeg-bin", opts.ffmpegBin, "ffmpeg command path")
	installCmd.Flags().StringVar(&opts.ttsBin, "tts-bin", opts.ttsBin, "text-to-speech command")
	installCmd.Flags().StringVar(&opts.label, "label", opts.label, "launchagent label")
	installCmd.Flags().StringVar(&opts.plistPath, "plist-path", opts.plistPath, "override launchagent plist path")
	installCmd.Flags().StringVar(&opts.stdoutLog, "stdout-log", opts.stdoutLog, "assistant stdout log file")
	installCmd.Flags().StringVar(&opts.stderrLog, "stderr-log", opts.stderrLog, "assistant stderr log file")
	installCmd.Flags().BoolVar(&opts.keepAlive, "keep-alive", opts.keepAlive, "set KeepAlive in launchagent")
	installCmd.Flags().BoolVar(&opts.runAtLoad, "run-at-load", opts.runAtLoad, "set RunAtLoad in launchagent")
	installCmd.Flags().BoolVar(&opts.installLoad, "load", opts.installLoad, "run launchctl load after install")

	cmd.AddCommand(startCmd, doctorCmd, installCmd)
	return cmd
}

func runAssistantCommand(ctx context.Context, opts assistantOptions, stdout, stderr io.Writer) error {
	switch strings.TrimSpace(opts.action) {
	case "start":
		return assistant.Start(ctx, assistant.StartOptions{
			ServerURL:    strings.TrimSpace(opts.serverURL),
			SessionID:    strings.TrimSpace(opts.sessionID),
			APIToken:     strings.TrimSpace(opts.apiToken),
			WorkspaceDir: strings.TrimSpace(opts.workspace),
			Hotkey:       strings.TrimSpace(opts.hotkey),
			AudioInput:   strings.TrimSpace(opts.audioInput),
			WhisperBin:   strings.TrimSpace(opts.whisperBin),
			WhisperModel: strings.TrimSpace(opts.whisperModel),
			FFmpegBin:    strings.TrimSpace(opts.ffmpegBin),
			TTSBin:       strings.TrimSpace(opts.ttsBin),
			Stdin:        os.Stdin,
			Stdout:       stdout,
			Stderr:       stderr,
		})
	case "doctor":
		report := assistant.RunDoctor(assistant.DoctorOptions{
			WhisperBinary: strings.TrimSpace(opts.whisperBin),
			FFmpegBinary:  strings.TrimSpace(opts.ffmpegBin),
			TTSBinary:     strings.TrimSpace(opts.ttsBin),
		})
		fmt.Fprintf(stdout, "assistant doctor ok=%t\n", report.OK)
		for _, check := range report.Checks {
			if check.Found {
				fmt.Fprintf(stdout, "- %s found=%t path=%s\n", check.Name, check.Found, check.Path)
				continue
			}
			fmt.Fprintf(stdout, "- %s found=%t error=%s\n", check.Name, check.Found, check.Error)
		}
		for _, note := range report.Notes {
			if strings.TrimSpace(note) == "" {
				continue
			}
			fmt.Fprintf(stdout, "note: %s\n", strings.TrimSpace(note))
		}
		return nil
	case "install-launchagent":
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("install-launchagent is only supported on macOS")
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		plistPath := strings.TrimSpace(opts.plistPath)
		if plistPath == "" {
			plistPath, err = assistant.DefaultLaunchAgentPath(strings.TrimSpace(opts.label))
			if err != nil {
				return err
			}
		}
		stdoutLog := strings.TrimSpace(opts.stdoutLog)
		stderrLog := strings.TrimSpace(opts.stderrLog)
		if stdoutLog == "" {
			stdoutLog = filepathJoinHome("Library/Logs/tars-assistant.out.log")
		}
		if stderrLog == "" {
			stderrLog = filepathJoinHome("Library/Logs/tars-assistant.err.log")
		}
		args := []string{
			exe, "assistant", "start",
			"--server-url", strings.TrimSpace(opts.serverURL),
			"--workspace-dir", strings.TrimSpace(opts.workspace),
			"--hotkey", strings.TrimSpace(opts.hotkey),
			"--audio-input", strings.TrimSpace(opts.audioInput),
			"--whisper-bin", strings.TrimSpace(opts.whisperBin),
			"--ffmpeg-bin", strings.TrimSpace(opts.ffmpegBin),
			"--tts-bin", strings.TrimSpace(opts.ttsBin),
		}
		if strings.TrimSpace(opts.whisperModel) != "" {
			args = append(args, "--whisper-model", strings.TrimSpace(opts.whisperModel))
		}
		if strings.TrimSpace(opts.sessionID) != "" {
			args = append(args, "--session", strings.TrimSpace(opts.sessionID))
		}
		if strings.TrimSpace(opts.apiToken) != "" {
			args = append(args, "--api-token", strings.TrimSpace(opts.apiToken))
		}
		content := assistant.BuildLaunchAgentPlist(assistant.LaunchAgentConfig{
			Label:            strings.TrimSpace(opts.label),
			ProgramArguments: args,
			WorkingDirectory: strings.TrimSpace(opts.workspace),
			StdoutPath:       stdoutLog,
			StderrPath:       stderrLog,
			KeepAlive:        opts.keepAlive,
			RunAtLoad:        opts.runAtLoad,
		})
		if err := assistant.InstallLaunchAgent(plistPath, content); err != nil {
			return err
		}
		if opts.installLoad {
			_ = exec.CommandContext(ctx, "launchctl", "unload", plistPath).Run()
			if out, err := exec.CommandContext(ctx, "launchctl", "load", plistPath).CombinedOutput(); err != nil {
				return fmt.Errorf("launchctl load failed: %w: %s", err, strings.TrimSpace(string(out)))
			}
		}
		fmt.Fprintf(stdout, "launchagent installed: %s\n", plistPath)
		return nil
	default:
		return fmt.Errorf("unsupported assistant action: %s", strings.TrimSpace(opts.action))
	}
}

func filepathJoinHome(rel string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return rel
	}
	return strings.TrimSpace(home) + "/" + strings.TrimLeft(strings.TrimSpace(rel), "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
