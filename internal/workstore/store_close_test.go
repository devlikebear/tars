package workstore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A transaction whose context is cancelled is rolled back by database/sql on
// its own goroutine, so the caller can return before the connection is handed
// back. sql.DB.Close only closes idle connections; one still in use closes
// later, whenever it is released. Close has to cover that window: on Windows a
// file that is still open cannot be removed, and the caller removing the
// ledger's directory right after Close failed with a sharing violation.
func TestCloseWaitsForInUseConnectionsToClose(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}

	closed := make(chan error, 1)
	go func() { closed <- store.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned (err=%v) while a transaction still held a connection", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after the transaction released its connection")
	}
	if open := store.db.Stats().OpenConnections; open != 0 {
		t.Fatalf("open connections after Close = %d, want 0", open)
	}
}

func TestCloseReportsConnectionsThatAreNeverReleased(t *testing.T) {
	previous := closeConnectionTimeout
	closeConnectionTimeout = 20 * time.Millisecond
	t.Cleanup(func() { closeConnectionTimeout = previous })

	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	err = store.Close()
	if err == nil || !strings.Contains(err.Error(), "1 database connection") {
		t.Fatalf("close with a leaked transaction err = %v, want the open connection reported", err)
	}
}
