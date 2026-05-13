package main

import (
	"fmt"
	"io"

	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/devlikebear/tars/internal/skillhub/sources/openclaw"
)

// fprintf is fmt.Fprintf with the return values discarded. Used for
// best-effort stdout writes (CLI status text) where keeping errcheck
// happy with explicit `_, _ =` would clutter the call sites.
func fprintf(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}

// fprintln is fmt.Fprintln with the return values discarded.
func fprintln(w io.Writer, a ...any) {
	_, _ = fmt.Fprintln(w, a...)
}

// fprint is fmt.Fprint with the return values discarded.
func fprint(w io.Writer, a ...any) {
	_, _ = fmt.Fprint(w, a...)
}

// newSkillInstaller returns a skillhub.Installer with the built-in tars-hub
// source plus every external hub adapter the CLI ships with. Wiring lives
// here (not inside the skillhub package) so internal/skillhub does not
// import its own subpackages.
func newSkillInstaller(workspaceDir string) *skillhub.Installer {
	inst := skillhub.NewInstaller(workspaceDir)
	_ = inst.Sources.Register(openclaw.New())
	return inst
}
