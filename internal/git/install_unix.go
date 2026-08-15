//go:build !windows

package git

// InstallRoots has nothing to offer on unix. Git is not distributed as a
// bundle carrying the other tools tars shells out to — those live at their own
// standard absolute paths or on PATH — so there is no install root to search.
func InstallRoots() []string { return nil }
