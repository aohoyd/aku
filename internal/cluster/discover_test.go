package cluster

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// writeKubeconfig writes a minimal but valid kubeconfig at path with the given
// context names.
func writeKubeconfig(t *testing.T, path string, contexts ...string) {
	t.Helper()
	var b []byte
	b = append(b, []byte("apiVersion: v1\nkind: Config\nclusters:\n- name: c1\n  cluster:\n    server: https://example.invalid\nusers:\n- name: u1\n  user: {}\ncontexts:\n")...)
	for _, ctx := range contexts {
		b = append(b, []byte("- name: "+ctx+"\n  context:\n    cluster: c1\n    user: u1\n")...)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(entries []ContextEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func fileFor(entries []ContextEntry, name string) string {
	for _, e := range entries {
		if e.Name == name {
			return e.File
		}
	}
	return ""
}

func TestScanKubeconfigs(t *testing.T) {
	root := t.TempDir()

	// (a) valid kubeconfig with 2 contexts at the top level
	top := filepath.Join(root, "top.yaml")
	writeKubeconfig(t, top, "alpha", "beta")

	// (b) nested subdir with another valid kubeconfig (tests recursion)
	nested := filepath.Join(root, "sub", "deep", "nested.yaml")
	writeKubeconfig(t, nested, "gamma")

	// (c) non-kubeconfig YAML file (must be skipped)
	if err := os.WriteFile(filepath.Join(root, "notes.yaml"), []byte("some: random\nyaml: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// (d) kubeconfig-shaped file with zero contexts (must be skipped)
	if err := os.WriteFile(filepath.Join(root, "empty.yaml"), []byte("apiVersion: v1\nkind: Config\ncontexts: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := ScanKubeconfigs([]string{root}, "")

	got := names(entries)
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("got contexts %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got contexts %v, want %v (sorted)", got, want)
		}
	}
	if f := fileFor(entries, "gamma"); f != nested {
		t.Fatalf("gamma should come from nested file %q, got %q", nested, f)
	}
}

func TestScanKubeconfigsDuplicateFirstWriterWins(t *testing.T) {
	root := t.TempDir()

	// Two files with the same context name "dup". Walk order over a single
	// directory is lexical by WalkDir, so a.yaml is visited before b.yaml.
	a := filepath.Join(root, "a.yaml")
	b := filepath.Join(root, "b.yaml")
	writeKubeconfig(t, a, "dup")
	writeKubeconfig(t, b, "dup")

	entries := ScanKubeconfigs([]string{root}, "")

	count := 0
	for _, e := range entries {
		if e.Name == "dup" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 entry for duplicate name, got %d", count)
	}
	if f := fileFor(entries, "dup"); f != a {
		t.Fatalf("first-writer-wins: dup should come from %q, got %q", a, f)
	}
}

func TestScanKubeconfigsDefaultWinsCollision(t *testing.T) {
	root := t.TempDir()

	// default kubeconfig defines "shared"; a dir file also defines "shared".
	def := filepath.Join(root, "default", "config")
	writeKubeconfig(t, def, "shared")

	dir := filepath.Join(root, "extras")
	dirFile := filepath.Join(dir, "other.yaml")
	writeKubeconfig(t, dirFile, "shared", "extra")

	entries := ScanKubeconfigs([]string{dir}, def)

	if f := fileFor(entries, "shared"); f != def {
		t.Fatalf("default path should win collision for 'shared': want %q, got %q", def, f)
	}
	if f := fileFor(entries, "extra"); f != dirFile {
		t.Fatalf("'extra' should come from dir file %q, got %q", dirFile, f)
	}
}

func TestScanKubeconfigsMissingDirSkipped(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good.yaml")
	writeKubeconfig(t, good, "ctx1")

	missing := filepath.Join(root, "does-not-exist")
	entries := ScanKubeconfigs([]string{missing, root}, "")
	if f := fileFor(entries, "ctx1"); f != good {
		t.Fatalf("ctx1 should be discovered from %q despite a missing dir, got %q", good, f)
	}
}

func TestScanKubeconfigsSorted(t *testing.T) {
	root := t.TempDir()
	// Write contexts in non-sorted order across files.
	writeKubeconfig(t, filepath.Join(root, "z.yaml"), "zoo")
	writeKubeconfig(t, filepath.Join(root, "m.yaml"), "mid", "aaa")

	entries := ScanKubeconfigs([]string{root}, "")

	got := names(entries)
	want := append([]string(nil), got...)
	sort.Strings(want)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("result not sorted by Name: got %v", got)
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 contexts, got %d (%v)", len(got), got)
	}
}

func TestScanKubeconfigsEmptyInputs(t *testing.T) {
	entries := ScanKubeconfigs(nil, "")
	if len(entries) != 0 {
		t.Fatalf("expected no entries for empty inputs, got %v", names(entries))
	}
}

// TestScanKubeconfigsZeroContextSkipped isolates the zero-context-skip case that
// is otherwise bundled inside TestScanKubeconfigs: a structurally-valid but
// context-less kubeconfig (the way non-kubeconfig YAML manifests after parsing)
// must contribute zero entries, leaving only the real kubeconfig's context. This
// pins the `select(.contexts | length != 0)` behavior on its own.
func TestScanKubeconfigsZeroContextSkipped(t *testing.T) {
	root := t.TempDir()
	writeKubeconfig(t, filepath.Join(root, "real.yaml"), "ctx-alpha")
	if err := os.WriteFile(filepath.Join(root, "empty.yaml"), []byte("apiVersion: v1\nkind: Config\ncontexts: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := ScanKubeconfigs([]string{root}, "")
	if len(entries) != 1 {
		t.Fatalf("expected only the real context, got %d: %+v", len(entries), entries)
	}
	if entries[0].Name != "ctx-alpha" {
		t.Errorf("got %q want %q", entries[0].Name, "ctx-alpha")
	}
}

// TestScanKubeconfigsDefaultPathIsDirectorySkipped verifies the defaultPath
// branch tolerates a path that is a directory (not a regular file): the os.Stat
// !IsDir() guard must skip it rather than attempting to parse it, so the scan
// still returns only the directory-discovered contexts.
func TestScanKubeconfigsDefaultPathIsDirectorySkipped(t *testing.T) {
	root := t.TempDir()
	writeKubeconfig(t, filepath.Join(root, "extra.yaml"), "extra-ctx")

	// Point defaultPath at a directory, not a file.
	dirAsDefault := t.TempDir()

	entries := ScanKubeconfigs([]string{root}, dirAsDefault)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (default dir skipped), got %d: %+v", len(entries), entries)
	}
	if entries[0].Name != "extra-ctx" {
		t.Errorf("got %q want %q", entries[0].Name, "extra-ctx")
	}
}
