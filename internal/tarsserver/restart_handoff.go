package tarsserver

import (
	"net"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// restartPredecessorEnv marks a process as the successor of a restart handoff.
// Only the Windows execRestart sets it: there the predecessor is still running
// (and still holding the API port) when the successor starts. The unix path
// replaces the process image, so the variable is never set and the wait below
// is skipped.
const restartPredecessorEnv = "TARS_RESTART_PREDECESSOR_PID"

// restartHandoffTimeout bounds the wait so a predecessor that refuses to die
// cannot wedge the successor indefinitely — better to attempt the bind and fail
// loudly with "address already in use" than to hang with no output.
const restartHandoffTimeout = 10 * time.Second

// restartHandoffPollInterval is short enough that a normal handoff (the
// predecessor exits within milliseconds of spawning us) costs one poll.
const restartHandoffPollInterval = 50 * time.Millisecond

// awaitPredecessorExit blocks until addr is bindable, when this process was
// started as the successor of a restart. It is a no-op otherwise.
//
// It probes the port rather than the predecessor's PID because the port is what
// actually blocks startup: the predecessor may have already exited while the
// socket is still being torn down.
func awaitPredecessorExit(addr string, logger zerolog.Logger) {
	pid := strings.TrimSpace(os.Getenv(restartPredecessorEnv))
	if pid == "" {
		return
	}
	// Clear it so a later restart of *this* process does not inherit the marker
	// and wait on a predecessor that is long gone.
	_ = os.Unsetenv(restartPredecessorEnv)

	deadline := time.Now().Add(restartHandoffTimeout)
	for {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			_ = ln.Close()
			logger.Info().Str("addr", addr).Str("predecessor_pid", pid).Msg("restart handoff complete")
			return
		}
		if time.Now().After(deadline) {
			logger.Warn().Str("addr", addr).Str("predecessor_pid", pid).Err(err).
				Msg("predecessor still holds the api port; binding anyway")
			return
		}
		time.Sleep(restartHandoffPollInterval)
	}
}
