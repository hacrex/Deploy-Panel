// Package boundary holds tests that enforce the module boundaries described in
// docs/MODULES.md. It contains no production code.
package boundary

import (
	"bufio"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// modulePath reads the module path from root/go.mod.
func modulePath(t *testing.T, root string) string {
	t.Helper()
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatal("no module line in go.mod")
	return ""
}

// violations returns one message per breach of the rules:
//  1. internal/core must not import mod/*.
//  2. a mod/<a> package must not import mod/<b>. Modules talk through core interfaces.
func violations(root, module string) ([]string, error) {
	var out []string
	check := func(dir string, forbidden func(importPath string) bool, rule string) error {
		return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return filepath.SkipAll
				}
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imp := range f.Imports {
				p := strings.Trim(imp.Path.Value, `"`)
				if forbidden(p) {
					rel, _ := filepath.Rel(root, path)
					out = append(out, rule+": "+rel+" imports "+p)
				}
			}
			return nil
		})
	}

	modPrefix := module + "/mod/"
	if err := check(filepath.Join(root, "internal", "core"),
		func(p string) bool { return strings.HasPrefix(p, modPrefix) },
		"core must not import modules"); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(filepath.Join(root, "mod"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		self := modPrefix + e.Name()
		err := check(filepath.Join(root, "mod", e.Name()),
			func(p string) bool {
				return strings.HasPrefix(p, modPrefix) && p != self && !strings.HasPrefix(p, self+"/")
			},
			"module "+e.Name()+" must not import another module")
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func TestRepositoryRespectsModuleBoundaries(t *testing.T) {
	root := filepath.Join("..", "..")
	v, err := violations(root, modulePath(t, root))
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range v {
		t.Error(msg)
	}
}

// The checker itself needs a test, otherwise it could pass for the wrong reason.
func TestCheckerDetectsViolations(t *testing.T) {
	root := t.TempDir()
	write := func(rel, src string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const m = "example.com/x"
	write("internal/core/site/a.go", "package site\nimport _ \""+m+"/mod/dns\"\n")
	write("mod/dns/a.go", "package dns\nimport _ \""+m+"/mod/mail\"\n")
	write("mod/mail/a.go", "package mail\nimport _ \""+m+"/internal/core/site\"\n")

	v, err := violations(root, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 2 {
		t.Fatalf("want 2 violations (core->mod, mod->mod), got %d: %v", len(v), v)
	}
}
