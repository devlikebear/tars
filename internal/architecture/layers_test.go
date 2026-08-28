package architecture_test

import (
	"encoding/json"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/architecture"
)

const modulePath = "github.com/devlikebear/tars"

// packageInfo is the slice of `go list -json` this test reads.
type packageInfo struct {
	ImportPath string
	Deps       []string
}

// loadPackages returns every package in the module with its full transitive
// dependency list.
//
// It shells out to `go list` rather than parsing imports itself because the
// rule is about the *transitive* closure: a core package that imports a
// shared helper which imports the server is just as much a violation as a
// direct import, and only the toolchain knows the whole graph.
func loadPackages(t *testing.T) map[string][]string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-json", "./...")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}

	deps := map[string][]string{}
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for decoder.More() {
		var info packageInfo
		if err := decoder.Decode(&info); err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		deps[info.ImportPath] = info.Deps
	}
	if len(deps) == 0 {
		t.Fatal("go list returned no packages")
	}
	return deps
}

func internalPath(name string) string { return modulePath + "/internal/" + name }

func setOf(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}

// appImportsIn reports which app packages appear in a dependency list.
func appImportsIn(deps []string, apps map[string]bool) []string {
	var found []string
	for _, dep := range deps {
		rest, ok := strings.CutPrefix(dep, modulePath+"/internal/")
		if !ok {
			continue
		}
		// Sub-packages count as their parent (skillhub/sources/... is skillhub).
		top := rest
		if idx := strings.Index(top, "/"); idx >= 0 {
			top = top[:idx]
		}
		if apps[top] {
			found = append(found, top)
		}
	}
	sort.Strings(found)
	return slicesCompact(found)
}

func slicesCompact(in []string) []string {
	out := in[:0]
	var last string
	for i, v := range in {
		if i == 0 || v != last {
			out = append(out, v)
		}
		last = v
	}
	return out
}

// TestCorePackagesDoNotImportAppPackages is the rule. A reverse import from
// the core layer to the app layer fails here, naming both packages.
func TestCorePackagesDoNotImportAppPackages(t *testing.T) {
	packages := loadPackages(t)
	apps := setOf(architecture.AppPackages)

	for _, core := range architecture.CorePackages {
		path := internalPath(core)
		deps, ok := packages[path]
		if !ok {
			t.Errorf("core package %q is listed in architecture.CorePackages but does not exist", core)
			continue
		}
		if violations := appImportsIn(deps, apps); len(violations) > 0 {
			t.Errorf(
				"layer violation: core package internal/%s imports app package(s) %s\n"+
					"  core may not depend on app. Either move the shared code down into core,\n"+
					"  invert the dependency, or reclassify the package in internal/architecture/layers.go\n"+
					"  with a note on why the layering changed.",
				core, strings.Join(violations, ", "),
			)
		}
	}
}

// TestPublicPackagesDoNotImportAppPackages keeps the published surface free of
// the server's storage and scheduling stack — the property #927 established
// and this is here to hold.
func TestPublicPackagesDoNotImportAppPackages(t *testing.T) {
	packages := loadPackages(t)
	apps := setOf(architecture.AppPackages)

	for path, deps := range packages {
		if !strings.HasPrefix(path, modulePath+"/pkg/") {
			continue
		}
		if violations := appImportsIn(deps, apps); len(violations) > 0 {
			t.Errorf(
				"layer violation: public package %s imports app package(s) %s\n"+
					"  pkg/* is the external API surface; importing an app package puts the\n"+
					"  server's storage and scheduling stack into every consumer's build.",
				strings.TrimPrefix(path, modulePath+"/"), strings.Join(violations, ", "),
			)
		}
	}
}

// TestEveryInternalPackageIsClassified makes adding a package a decision.
// A new internal/ package fails this test until it is placed in one of the
// three lists, which is the point: the layering cannot drift by omission.
func TestEveryInternalPackageIsClassified(t *testing.T) {
	packages := loadPackages(t)
	classified := setOf(append(append(
		append([]string{}, architecture.CorePackages...),
		architecture.AppPackages...),
		architecture.SharedPackages...))

	seen := map[string]bool{}
	for path := range packages {
		rest, ok := strings.CutPrefix(path, modulePath+"/internal/")
		if !ok {
			continue
		}
		top := rest
		if idx := strings.Index(top, "/"); idx >= 0 {
			top = top[:idx]
		}
		if seen[top] {
			continue
		}
		seen[top] = true
		if !classified[top] {
			t.Errorf(
				"internal/%s is not classified. Add it to CorePackages, AppPackages, or\n"+
					"  SharedPackages in internal/architecture/layers.go — the choice decides\n"+
					"  what it is allowed to import.",
				top,
			)
		}
	}
}

// TestClassificationListsAreDisjointAndReal catches a package listed twice, or
// listed but deleted, which would silently weaken the other tests.
func TestClassificationListsAreDisjointAndReal(t *testing.T) {
	packages := loadPackages(t)

	existing := map[string]bool{}
	for path := range packages {
		if rest, ok := strings.CutPrefix(path, modulePath+"/internal/"); ok {
			top := rest
			if idx := strings.Index(top, "/"); idx >= 0 {
				top = top[:idx]
			}
			existing[top] = true
		}
	}

	seen := map[string]string{}
	for listName, list := range map[string][]string{
		"CorePackages":   architecture.CorePackages,
		"AppPackages":    architecture.AppPackages,
		"SharedPackages": architecture.SharedPackages,
	} {
		for _, name := range list {
			if other, dup := seen[name]; dup {
				t.Errorf("internal/%s is listed in both %s and %s", name, other, listName)
			}
			seen[name] = listName
			if !existing[name] {
				t.Errorf("%s lists internal/%s, which does not exist", listName, name)
			}
		}
	}
}
