package execsandbox

// BubblewrapArgs renders p as bwrap arguments that run argv in dir.
//
// The whole filesystem is mounted read-only, then each writable path is bound
// read-write over it, read-only paths are bound read-only again inside those,
// and hidden paths are covered last: an empty tmpfs over a directory,
// /dev/null over a file. Mount order is the precedence, the same as the rule
// order in SeatbeltProfile. bwrap can only mount over a path that exists, so
// Missing paths are skipped — a limit Seatbelt does not have.
//
// --unshare-pid with --die-with-parent ties every process the command starts
// to bwrap itself, so killing bwrap (the exec tool kills its process group on
// timeout) ends them all. Without network, --unshare-net leaves only the
// namespace's own loopback.
func BubblewrapArgs(p Policy, dir string, argv []string) []string {
	args := []string{
		"--die-with-parent",
		"--unshare-pid",
		"--new-session",
	}
	if !p.Network {
		args = append(args, "--unshare-net")
	}
	args = append(args,
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--proc", "/proc",
	)
	for _, w := range p.Writable {
		if w.Kind != Missing {
			args = append(args, "--bind", w.Path, w.Path)
		}
	}
	for _, r := range p.ReadOnly {
		if r.Kind != Missing {
			args = append(args, "--ro-bind", r.Path, r.Path)
		}
	}
	for _, h := range p.Hidden {
		switch h.Kind {
		case Dir:
			args = append(args, "--tmpfs", h.Path, "--remount-ro", h.Path)
		case File:
			args = append(args, "--ro-bind", "/dev/null", h.Path)
		}
	}
	args = append(args, "--chdir", dir, "--")
	return append(args, argv...)
}
