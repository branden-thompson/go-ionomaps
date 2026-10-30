//go:build mutants

package ionomaps_test

// mutants_kill_test.go — the costly half (watchpost RL-5), run by scripts/gate's
// full lane: each mutant applied to a copy of the tree, never to the tree
// itself, so a run touches nothing a person is editing; one copy for every
// mutant, each file put back from its saved bytes after its mutant. A mutant
// counts only when the copy still compiles; its test must then fail.

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// treeCopy copies the files git knows of, tracked or new and not ignored,
// into a temporary directory: the tree as it stands, without git's own.
func treeCopy(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		t.Fatalf("listing the tree: %v", err)
	}
	dst := t.TempDir()
	for _, name := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if name == "" {
			continue
		}
		info, err := os.Lstat(name)
		if err != nil || !info.Mode().IsRegular() {
			continue // deleted in the tree, or not a plain file
		}
		target := filepath.Join(dst, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		copyFile(t, name, target)
	}
	return dst
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	in, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }() // read only
	out, err := os.Create(to)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// goIn runs the go command in dir: its combined output and whether it
// succeeded.
func goIn(dir string, args ...string) (string, bool) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	ok := cmd.Run() == nil // run before the output is read
	return buf.String(), ok
}

// TestEveryMutantIsKilled applies each mutant to the copy of the tree in
// turn. The copy must build and vet with the mutant in place - a mutant that
// does not compile is no evidence either way - and the mutant's test must
// then fail in its package. A file that cannot be put back ends the run: the
// copy would no longer be the tree.
func TestEveryMutantIsKilled(t *testing.T) {
	dir := treeCopy(t)
	for _, m := range loadMutants(t) {
		path := filepath.Join(dir, m.File)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(m.ID, func(t *testing.T) {
			if strings.Count(string(b), m.Old) != 1 {
				t.Fatalf("its text is not in %s once", m.File)
			}
			if err := os.WriteFile(path, []byte(strings.Replace(string(b), m.Old, m.New, 1)), 0o644); err != nil {
				t.Fatal(err)
			}
			if out, ok := goIn(dir, "vet", m.Package); !ok {
				t.Fatalf("INVALID: with the mutant in place the package does not build or vet, so it is no evidence:\n%s", out)
			}
			out, ok := goIn(dir, "test", "-count=1", "-run", "^"+m.Test+"$", m.Package)
			if ok || !strings.Contains(out, "--- FAIL: "+m.Test) {
				t.Errorf("SURVIVED: %s (%s, %s) - %s still passes with it in place:\n%s", m.ID, m.Row, m.What, m.Test, out)
			}
		})
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatalf("%s's file was not put back: %v", m.ID, err)
		}
		if back, err := os.ReadFile(path); err != nil || !bytes.Equal(back, b) {
			t.Fatalf("%s's file is not as it was after putting it back", m.ID)
		}
	}
}
