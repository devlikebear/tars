package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const assistantTokenSentinel = "sentinel-assistant-token-4d2b"

func installAssistantForTest(t *testing.T, opts assistantOptions) (plist string, stdout string) {
	t.Helper()
	originalGOOS := assistantRuntimeGOOS
	t.Cleanup(func() { assistantRuntimeGOOS = originalGOOS })
	assistantRuntimeGOOS = "darwin"

	plistPath := filepath.Join(t.TempDir(), "io.tars.assistant.test.plist")
	opts.action = "install-launchagent"
	opts.serverURL = "http://127.0.0.1:43180"
	opts.workspace = t.TempDir()
	opts.label = "io.tars.assistant.test"
	opts.plistPath = plistPath
	opts.installLoad = false
	var out bytes.Buffer
	if err := runAssistantCommand(context.Background(), opts, &out, io.Discard); err != nil {
		t.Fatalf("install: %v", err)
	}
	data, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), out.String()
}

// The plist used to carry "--api-token <token>": visible in ps and in a
// world-readable file. It must now hold only the path of a 0600 file.
func TestInstallLaunchAgentKeepsTheTokenOutOfThePlist(t *testing.T) {
	configDir := t.TempDir()
	original := assistantUserConfigDir
	t.Cleanup(func() { assistantUserConfigDir = original })
	assistantUserConfigDir = func() (string, error) { return configDir, nil }

	plist, out := installAssistantForTest(t, assistantOptions{apiToken: assistantTokenSentinel})

	if strings.Contains(plist, assistantTokenSentinel) {
		t.Fatalf("plist contains the token:\n%s", plist)
	}
	if strings.Contains(plist, "<string>--api-token</string>") {
		t.Fatalf("plist still passes --api-token:\n%s", plist)
	}
	tokenPath := filepath.Join(configDir, "tars-assistant", "api-token")
	if !strings.Contains(plist, "<string>--api-token-file</string>") || !strings.Contains(plist, "<string>"+tokenPath+"</string>") {
		t.Fatalf("plist does not point at the token file %s:\n%s", tokenPath, plist)
	}
	if strings.Contains(out, assistantTokenSentinel) || !strings.Contains(out, tokenPath) {
		t.Fatalf("install output = %q", out)
	}

	got, err := readAssistantToken(tokenPath)
	if err != nil || got != assistantTokenSentinel {
		t.Fatalf("token file holds %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{tokenPath: 0o600, filepath.Dir(tokenPath): 0o700} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if mode := info.Mode().Perm(); mode != want {
				t.Errorf("%s mode = %o, want %o", path, mode, want)
			}
		}
	}
}

func TestInstallLaunchAgentHonorsAnExplicitTokenFile(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "nested", "token")
	plist, _ := installAssistantForTest(t, assistantOptions{apiToken: assistantTokenSentinel, apiTokenFile: tokenPath})
	if !strings.Contains(plist, "<string>"+tokenPath+"</string>") {
		t.Fatalf("plist does not use the given path:\n%s", plist)
	}
	if got, err := readAssistantToken(tokenPath); err != nil || got != assistantTokenSentinel {
		t.Fatalf("token file holds %q, %v", got, err)
	}
}

func TestInstallLaunchAgentWithoutATokenWritesNoFile(t *testing.T) {
	configDir := t.TempDir()
	original := assistantUserConfigDir
	t.Cleanup(func() { assistantUserConfigDir = original })
	assistantUserConfigDir = func() (string, error) { return configDir, nil }

	plist, _ := installAssistantForTest(t, assistantOptions{})
	if strings.Contains(plist, "api-token") {
		t.Fatalf("plist mentions a token without one:\n%s", plist)
	}
	if _, err := os.Stat(filepath.Join(configDir, "tars-assistant")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("token directory created without a token: %v", err)
	}
}

func TestInstallLaunchAgentFailsWhenTheTokenCannotBeStored(t *testing.T) {
	original := assistantUserConfigDir
	t.Cleanup(func() { assistantUserConfigDir = original })
	assistantUserConfigDir = func() (string, error) { return "", errors.New("no config dir") }

	originalGOOS := assistantRuntimeGOOS
	t.Cleanup(func() { assistantRuntimeGOOS = originalGOOS })
	assistantRuntimeGOOS = "darwin"
	plistPath := filepath.Join(t.TempDir(), "x.plist")
	err := runAssistantCommand(context.Background(), assistantOptions{
		action: "install-launchagent", apiToken: assistantTokenSentinel,
		label: "io.tars.assistant.test", plistPath: plistPath, workspace: t.TempDir(),
	}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "user config directory") {
		t.Fatalf("want a config-dir error, got %v", err)
	}
	if _, statErr := os.Stat(plistPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("plist written although the token could not be stored")
	}
}

func TestResolveAssistantAPIToken(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good")
	if err := writeAssistantToken(good, "from-file"); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		opts    assistantOptions
		want    string
		wantErr string
	}{
		{name: "flag or env wins", opts: assistantOptions{apiToken: " direct ", apiTokenFile: good}, want: "direct"},
		{name: "file", opts: assistantOptions{apiTokenFile: good}, want: "from-file"},
		{name: "none", opts: assistantOptions{}, want: ""},
		{name: "missing file", opts: assistantOptions{apiTokenFile: filepath.Join(dir, "missing")}, wantErr: "read assistant token file"},
		{name: "empty file", opts: assistantOptions{apiTokenFile: empty}, wantErr: "empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveAssistantAPIToken(c.opts)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("want error %q, got %q, %v", c.wantErr, got, err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

func TestAssistantStartFailsOnAnUnreadableTokenFile(t *testing.T) {
	err := runAssistantCommand(context.Background(), assistantOptions{
		action: "start", apiTokenFile: filepath.Join(t.TempDir(), "missing"),
	}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "read assistant token file") {
		t.Fatalf("want token file error before starting, got %v", err)
	}
}

func TestDefaultAssistantTokenPath(t *testing.T) {
	original := assistantUserConfigDir
	t.Cleanup(func() { assistantUserConfigDir = original })
	assistantUserConfigDir = func() (string, error) { return "/cfg", nil }
	got, err := defaultAssistantTokenPath()
	if err != nil || got != filepath.Join("/cfg", "tars-assistant", "api-token") {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestWriteAssistantTokenErrors(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A regular file where the directory should be.
	if err := writeAssistantToken(filepath.Join(blocker, "token"), "t"); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("want directory error, got %v", err)
	}
	// A directory where the token file should be. It holds a file: on
	// Windows atomicwrite falls back to removing the destination and
	// renaming again, which succeeds over an empty directory.
	target := filepath.Join(dir, "asdir")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeAssistantToken(target, "t"); err == nil {
		t.Fatal("want write error over a directory")
	}
}
