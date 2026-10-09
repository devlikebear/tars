package workstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// A query whose context is cancelled just as it produces its first row must
// not keep the database file open past Close. The sqlite driver used to drop
// the rows of such a query without finalizing their statement, and
// sqlite3_close_v2 keeps a connection with an unfinalized statement alive: the
// pool counted the connection as closed while its file handle stayed open for
// the rest of the process. The scheduler polls with a context that is
// cancelled at shutdown, so on Windows removing the ledger's directory after
// Close failed with "being used by another process" however long the caller
// waited.
func TestCloseReleasesTheFileAfterQueriesCancelledMidFlight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")
	store := openTestStore(t, path)

	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				ctx, cancel := context.WithCancel(context.Background())
				go cancel()
				rows, err := store.db.QueryContext(ctx, "SELECT 1")
				if err == nil {
					_ = rows.Close()
				}
				cancel()
			}
		}()
	}
	wg.Wait()

	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if held, checked := openHandlesOn(t, path); checked && held != 0 {
		t.Fatalf("%d handle(s) on the database file still open after Close", held)
	}
	// Windows refuses to remove a file that is still open.
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s after Close: %v", filepath.Base(name), err)
		}
	}
}

// openHandlesOn counts this process's open handles on path. checked is false
// where there is no cheap way to list them (Windows: the removal above is the
// check there).
func openHandlesOn(t *testing.T, path string) (held int, checked bool) {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}
	if entries, err := os.ReadDir("/proc/self/fd"); err == nil {
		for _, entry := range entries {
			target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
			if err == nil && strings.HasPrefix(target, resolved) {
				held++
			}
		}
		return held, true
	}
	if runtime.GOOS != "darwin" {
		return 0, false
	}
	lsof, err := exec.LookPath("lsof")
	if err != nil {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// lsof exits non-zero when it lists nothing, so only its output counts.
	out, _ := exec.CommandContext(ctx, lsof, "-p", strconv.Itoa(os.Getpid()), "-Fn").Output()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") && strings.HasPrefix(line[1:], resolved) {
			held++
		}
	}
	return held, true
}
