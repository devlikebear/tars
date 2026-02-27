package assistant

import (
	"errors"
	"testing"
)

func TestRunDoctorReportsMissingBinaries(t *testing.T) {
	report := RunDoctor(DoctorOptions{
		LookupPath: func(name string) (string, error) {
			switch name {
			case "say":
				return "/usr/bin/say", nil
			default:
				return "", errors.New("not found")
			}
		},
		WhisperBinary: "whisper-cli",
		FFmpegBinary:  "ffmpeg",
		TTSBinary:     "say",
	})

	if len(report.Missing) != 2 {
		t.Fatalf("expected 2 missing binaries, got %+v", report.Missing)
	}
	if report.OK {
		t.Fatalf("expected doctor report OK=false when dependencies are missing")
	}
}
