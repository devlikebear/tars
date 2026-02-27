package assistant

import (
	"os/exec"
	"strings"
)

type DoctorOptions struct {
	LookupPath    func(file string) (string, error)
	WhisperBinary string
	FFmpegBinary  string
	TTSBinary     string
}

type DoctorCheck struct {
	Name  string `json:"name"`
	Path  string `json:"path,omitempty"`
	Found bool   `json:"found"`
	Error string `json:"error,omitempty"`
}

type DoctorReport struct {
	OK      bool          `json:"ok"`
	Checks  []DoctorCheck `json:"checks"`
	Missing []string      `json:"missing,omitempty"`
}

func RunDoctor(opts DoctorOptions) DoctorReport {
	lookup := opts.LookupPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	whisper := strings.TrimSpace(opts.WhisperBinary)
	if whisper == "" {
		whisper = "whisper-cli"
	}
	ffmpeg := strings.TrimSpace(opts.FFmpegBinary)
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	tts := strings.TrimSpace(opts.TTSBinary)
	if tts == "" {
		tts = "say"
	}
	bins := []string{whisper, ffmpeg, tts}
	checks := make([]DoctorCheck, 0, len(bins))
	missing := make([]string, 0)
	for _, name := range bins {
		resolved, err := lookup(name)
		check := DoctorCheck{Name: name}
		if err != nil {
			check.Found = false
			check.Error = err.Error()
			missing = append(missing, name)
		} else {
			check.Found = true
			check.Path = strings.TrimSpace(resolved)
		}
		checks = append(checks, check)
	}
	return DoctorReport{
		OK:      len(missing) == 0,
		Checks:  checks,
		Missing: missing,
	}
}
