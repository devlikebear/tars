// Command apisnapshot prints the exported API surface of the public pkg/*
// packages, one identifier per line, in a stable order.
//
// The output is checked in as docs/public-api-surface.txt and diffed in CI.
// A changed snapshot fails until the file is regenerated in the same PR,
// which makes a break to an external consumer visible in review rather than
// discovered downstream. It does not prevent the change — the point is that
// someone saw it.
//
//	go run ./cmd/apisnapshot            # print to stdout
//	go run ./cmd/apisnapshot -write     # update the checked-in snapshot
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const snapshotPath = "docs/public-api-surface.txt"

func main() {
	write := flag.Bool("write", false, "rewrite "+snapshotPath+" instead of printing")
	root := flag.String("root", ".", "module root")
	flag.Parse()

	lines, err := collect(filepath.Join(*root, "pkg"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "apisnapshot:", err)
		os.Exit(1)
	}

	body := header() + strings.Join(lines, "\n") + "\n"
	if !*write {
		fmt.Print(body)
		return
	}
	out := filepath.Join(*root, snapshotPath)
	if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "apisnapshot:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d identifiers)\n", out, len(lines))
}

func header() string {
	return "# Public API surface of pkg/*. Generated — do not edit by hand.\n" +
		"#\n" +
		"# Regenerate with:  make api-snapshot\n" +
		"# A diff here is a change to what external consumers can rely on.\n" +
		"# See docs/public-agent-packages.md for the stability policy.\n\n"
}

// collect walks every package directory under root and returns one sorted
// line per exported declaration.
func collect(root string) ([]string, error) {
	var lines []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		pkgLines, err := collectDir(path, root)
		if err != nil {
			return err
		}
		lines = append(lines, pkgLines...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(lines)
	return lines, nil
}

func collectDir(dir, root string) ([]string, error) {
	rel, err := filepath.Rel(filepath.Dir(root), dir)
	if err != nil {
		return nil, err
	}
	prefix := filepath.ToSlash(rel)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	// Parse each file individually rather than with parser.ParseDir, which is
	// deprecated for not honouring build tags. Files are read in sorted order
	// and the result is sorted again by the caller, so output stays stable.
	fset := token.NewFileSet()
	var lines []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		// Tests are not API.
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, err
		}
		lines = append(lines, declLines(prefix, file)...)
	}
	return lines, nil
}

func declLines(prefix string, file *ast.File) []string {
	var lines []string
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if !d.Name.IsExported() {
				continue
			}
			if d.Recv == nil {
				lines = append(lines, fmt.Sprintf("%s func %s", prefix, d.Name.Name))
				continue
			}
			recv := receiverName(d.Recv)
			// A method on an unexported type is not reachable.
			if recv == "" || !ast.IsExported(recv) {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s method %s.%s", prefix, recv, d.Name.Name))
		case *ast.GenDecl:
			lines = append(lines, genDeclLines(prefix, d)...)
		}
	}
	return lines
}

func genDeclLines(prefix string, decl *ast.GenDecl) []string {
	var lines []string
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if !s.Name.IsExported() {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s type %s", prefix, s.Name.Name))
			lines = append(lines, fieldLines(prefix, s)...)
		case *ast.ValueSpec:
			kind := "var"
			if decl.Tok == token.CONST {
				kind = "const"
			}
			for _, name := range s.Names {
				if !name.IsExported() {
					continue
				}
				lines = append(lines, fmt.Sprintf("%s %s %s", prefix, kind, name.Name))
			}
		}
	}
	return lines
}

// fieldLines records exported struct fields and interface methods, because a
// removed or renamed field breaks a consumer just as surely as a removed
// function — and that is exactly what the alias facade used to hide.
func fieldLines(prefix string, spec *ast.TypeSpec) []string {
	var lines []string
	switch t := spec.Type.(type) {
	case *ast.StructType:
		for _, field := range t.Fields.List {
			for _, name := range field.Names {
				if name.IsExported() {
					lines = append(lines, fmt.Sprintf("%s field %s.%s", prefix, spec.Name.Name, name.Name))
				}
			}
		}
	case *ast.InterfaceType:
		for _, method := range t.Methods.List {
			for _, name := range method.Names {
				if name.IsExported() {
					lines = append(lines, fmt.Sprintf("%s method %s.%s", prefix, spec.Name.Name, name.Name))
				}
			}
		}
	}
	return lines
}

func receiverName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	switch t := recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}
