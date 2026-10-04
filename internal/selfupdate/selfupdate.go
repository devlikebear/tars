// Package selfupdate replaces a release install of tars with the latest
// GitHub release, the way install.ps1 and install.sh install it, and restarts
// a running server onto the new binary.
//
// The flow is Check, then Install, then RestartServer. Install downloads the
// platform's server archive (internal/release.AssetArchiveName), verifies it
// against the release's checksums.txt, and swaps the executable: the running
// file is renamed to <exe>.old first, because Windows refuses to overwrite a
// running executable but allows renaming it. The .old file is removed by the
// next Install. A server keeps running the old code until it restarts, and
// POST /v1/admin/restart re-executes the path it was started from, which now
// holds the new binary.
//
// Everything here is platform-neutral and takes its platform as input, so the
// Windows behaviour is tested on every CI host.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/release"
)

const (
	// DefaultRepository is where releases are published.
	DefaultRepository = "devlikebear/tars"
	// DefaultAPIBaseURL is GitHub's REST API.
	DefaultAPIBaseURL = "https://api.github.com"
	checksumsAsset    = "checksums.txt"
	// maxArchiveBytes bounds a download; a server archive is ~40 MB.
	maxArchiveBytes = 512 << 20
)

var (
	// ErrDevelopmentBuild means the running binary has no release version to
	// compare against, such as a `make build` without a VERSION.
	ErrDevelopmentBuild = errors.New("this tars is a development build and does not update itself")
	// ErrHomebrew means Homebrew owns the binary and must update it.
	ErrHomebrew = errors.New("this tars is managed by Homebrew; update it with: brew upgrade devlikebear/tap/tars")
	// ErrNoAsset means the latest release has no archive for this platform.
	ErrNoAsset = errors.New("the latest release has no archive for this platform")
)

// Options say what is running and where releases come from.
type Options struct {
	// CurrentVersion is the running build's version (buildinfo.Version).
	CurrentVersion string
	// GOOS and GOARCH pick the archive.
	GOOS, GOARCH string
	// ExePath is the tars executable to replace, symlinks resolved.
	ExePath string
	// Repository defaults to DefaultRepository.
	Repository string
	// APIBaseURL defaults to DefaultAPIBaseURL.
	APIBaseURL string
	// HTTPClient defaults to a client with a five-minute timeout.
	HTTPClient *http.Client
}

// Release is the latest release's archive for this platform.
type Release struct {
	Version      string
	ArchiveName  string
	ArchiveURL   string
	ChecksumsURL string
}

// Status is what Check found.
type Status struct {
	Current   string  `json:"current"`
	Latest    string  `json:"latest"`
	Available bool    `json:"available"`
	Release   Release `json:"-"`
}

func (o Options) client() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func (o Options) apiBase() string {
	if base := strings.TrimRight(strings.TrimSpace(o.APIBaseURL), "/"); base != "" {
		return base
	}
	return DefaultAPIBaseURL
}

func (o Options) repository() string {
	if repo := strings.TrimSpace(o.Repository); repo != "" {
		return repo
	}
	return DefaultRepository
}

// Updatable reports whether a build may update itself: a development build
// has no release version to compare against.
func Updatable(version string) bool {
	v, ok := parseVersion(version)
	return ok && v != [3]int{}
}

// ManagedByHomebrew reports whether exePath is inside a Homebrew prefix.
func ManagedByHomebrew(exePath string) bool {
	p := filepath.ToSlash(exePath)
	return strings.Contains(p, "/Cellar/") || strings.Contains(p, "/homebrew/") || strings.Contains(p, "/linuxbrew/")
}

// Newer reports whether candidate is a later release than current.
func Newer(candidate, current string) bool {
	c, ok := parseVersion(candidate)
	if !ok {
		return false
	}
	cur, ok := parseVersion(current)
	if !ok {
		return false
	}
	for i := range c {
		if c[i] != cur[i] {
			return c[i] > cur[i]
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Check finds the latest release and whether it is newer than the running
// build.
func Check(ctx context.Context, opts Options) (Status, error) {
	if !Updatable(opts.CurrentVersion) {
		return Status{}, ErrDevelopmentBuild
	}
	if ManagedByHomebrew(opts.ExePath) {
		return Status{}, ErrHomebrew
	}
	rel, err := Latest(ctx, opts)
	if err != nil {
		return Status{}, err
	}
	current := strings.TrimPrefix(strings.TrimSpace(opts.CurrentVersion), "v")
	return Status{
		Current:   current,
		Latest:    rel.Version,
		Available: Newer(rel.Version, current),
		Release:   rel,
	}, nil
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Latest reads the latest release and picks this platform's server archive.
func Latest(ctx context.Context, opts Options) (Release, error) {
	url := opts.apiBase() + "/repos/" + opts.repository() + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "tars-update")
	resp, err := opts.client().Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("read latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("read latest release: %s", resp.Status)
	}
	var gh githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&gh); err != nil {
		return Release{}, fmt.Errorf("decode latest release: %w", err)
	}
	version := strings.TrimPrefix(strings.TrimSpace(gh.TagName), "v")
	if _, ok := parseVersion(version); !ok {
		return Release{}, fmt.Errorf("latest release tag %q is not a version", gh.TagName)
	}
	rel := Release{Version: version, ArchiveName: release.AssetArchiveName(version, opts.GOOS, opts.GOARCH)}
	for _, a := range gh.Assets {
		switch a.Name {
		case rel.ArchiveName:
			rel.ArchiveURL = a.URL
		case checksumsAsset:
			rel.ChecksumsURL = a.URL
		}
	}
	if rel.ArchiveURL == "" {
		return Release{}, fmt.Errorf("%w (%s, looked for %s)", ErrNoAsset, gh.TagName, rel.ArchiveName)
	}
	if rel.ChecksumsURL == "" {
		return Release{}, fmt.Errorf("release %s has no %s", gh.TagName, checksumsAsset)
	}
	return rel, nil
}

// Install downloads rel, verifies it, and puts its tars executable (and
// share/tars, when the archive has one) in place of the running install.
func Install(ctx context.Context, opts Options, rel Release) error {
	exe := strings.TrimSpace(opts.ExePath)
	if exe == "" {
		return errors.New("the tars executable path is unknown")
	}
	if ManagedByHomebrew(exe) {
		return ErrHomebrew
	}
	dir := filepath.Dir(exe)
	// A previous update's old binary: gone once nothing runs it any more.
	_ = os.Remove(exe + ".old")

	// Stage beside the executable so the final renames stay on one volume.
	stage, err := os.MkdirTemp(dir, ".tars-update-")
	if err != nil {
		return fmt.Errorf("create staging folder next to %s: %w", exe, err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	sums, err := fetchText(ctx, opts.client(), rel.ChecksumsURL)
	if err != nil {
		return err
	}
	want, err := checksumFor(sums, rel.ArchiveName)
	if err != nil {
		return err
	}
	archive := filepath.Join(stage, rel.ArchiveName)
	got, err := download(ctx, opts.client(), rel.ArchiveURL, archive)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", rel.ArchiveName, want, got)
	}

	binName := "tars"
	if opts.GOOS == "windows" {
		binName = "tars.exe"
	}
	extracted := filepath.Join(stage, "x")
	if err := extract(archive, extracted, binName); err != nil {
		return err
	}
	newBin := filepath.Join(extracted, binName)
	if _, err := os.Stat(newBin); err != nil {
		return fmt.Errorf("%s has no %s", rel.ArchiveName, binName)
	}
	if err := replaceFile(exe, newBin); err != nil {
		return err
	}
	if share := filepath.Join(extracted, "share", "tars"); isDir(share) {
		if err := replaceDir(shareTarget(exe, opts.GOOS), share); err != nil {
			return fmt.Errorf("installed %s but not its share/tars: %w", binName, err)
		}
	}
	if opts.GOOS != "windows" {
		// Unix lets a running file be unlinked; nothing waits on the copy.
		_ = os.Remove(exe + ".old")
	}
	return nil
}

// shareTarget is the share/tars the installers wrote: next to the executable
// on Windows (install.ps1) or when one is already there, otherwise in the
// prefix above bin/ (install.sh). internal/assetpath looks in both.
func shareTarget(exe, goos string) string {
	dir := filepath.Dir(exe)
	beside := filepath.Join(dir, "share", "tars")
	if goos == "windows" || isDir(beside) {
		return beside
	}
	return filepath.Join(filepath.Dir(dir), "share", "tars")
}

// replaceFile moves exe aside to exe.old and newBin into its place, putting
// exe back if the second rename fails.
func replaceFile(exe, newBin string) error {
	info, err := os.Stat(exe)
	if err != nil {
		return fmt.Errorf("stat %s: %w", exe, err)
	}
	if err := os.Chmod(newBin, info.Mode().Perm()|0o111); err != nil {
		return err
	}
	old := exe + ".old"
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("move %s aside (is %s still in use?): %w", exe, old, err)
	}
	if err := os.Rename(newBin, exe); err != nil {
		if restoreErr := os.Rename(old, exe); restoreErr != nil {
			return fmt.Errorf("install %s: %w (and restoring the old binary failed: %v; it is at %s)", exe, err, restoreErr, old)
		}
		return fmt.Errorf("install %s: %w", exe, err)
	}
	return nil
}

func replaceDir(target, source string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Rename(source, target)
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func fetchText(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "tars-update")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	return string(b), nil
}

// download writes url to dest and returns its sha256.
func download(ctx context.Context, client *http.Client, url, dest string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "tars-update")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxArchiveBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		return "", fmt.Errorf("download %s: %w", url, copyErr)
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n > maxArchiveBytes {
		return "", fmt.Errorf("download %s: larger than %d bytes", url, maxArchiveBytes)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// checksumFor reads name's sha256 from a `shasum -a 256` listing.
func checksumFor(sums, name string) (string, error) {
	sc := bufio.NewScanner(strings.NewReader(sums))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("%s has no entry for %s", checksumsAsset, name)
}

// extract unpacks the executable binName and anything under share/ from a
// .zip or .tar.gz into dest, refusing entries that would land outside it.
func extract(archive, dest, binName string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	if strings.HasSuffix(archive, ".zip") {
		return extractZip(archive, dest, binName)
	}
	return extractTarGz(archive, dest, binName)
}

// wanted maps an archive entry to its path under dest, or "" to skip it.
func wanted(name, binName string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(name, `\`, "/"))
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, ":") {
		return "", fmt.Errorf("archive entry %q is outside the archive", name)
	}
	if clean == binName || clean == "share" || strings.HasPrefix(clean, "share/") {
		return filepath.FromSlash(clean), nil
	}
	return "", nil
}

func extractZip(archive, dest, binName string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Base(archive), err)
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		rel, err := wanted(f.Name, binName)
		if err != nil {
			return err
		}
		if rel == "" {
			continue
		}
		// Checked again where the path is used: wanted refuses an escaping
		// name, and this refuses any joined path that is not under dest.
		target := filepath.Join(dest, rel)
		if strings.Contains(f.Name, "..") || !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("archive entry %q is outside the archive", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeFile(target, rc, f.Mode())
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(archive, dest, binName string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Base(archive), err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", filepath.Base(archive), err)
		}
		rel, err := wanted(hdr.Name, binName)
		if err != nil {
			return err
		}
		if rel == "" {
			continue
		}
		target := filepath.Join(dest, rel)
		if strings.Contains(hdr.Name, "..") || !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("archive entry %q is outside the archive", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(target, tr, hdr.FileInfo().Mode()); err != nil {
				return err
			}
		default:
			// Links and devices have no place in a release archive.
			return fmt.Errorf("archive entry %q is not a regular file", hdr.Name)
		}
	}
}

func writeFile(target string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	perm := mode.Perm()
	if perm == 0 {
		perm = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(r, maxArchiveBytes)); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
