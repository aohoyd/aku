package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildPath(t *testing.T) {
	base := t.TempDir()
	now := time.Date(2026, 5, 18, 14, 30, 45, 123_000_000, time.UTC)

	tests := []struct {
		name        string
		cluster     string
		ns          string
		pod         string
		container   string
		wantCluster string
		wantName    string
	}{
		{
			name:        "plain names",
			cluster:     "prod",
			ns:          "default",
			pod:         "nginx-abc",
			container:   "nginx",
			wantCluster: "prod",
			wantName:    "default-nginx-abc-nginx-2026-05-18T14-30-45.123.log",
		},
		{
			name:        "names with slashes are sanitized",
			cluster:     "arn:aws:eks:us-east-1:1234/cluster/foo",
			ns:          "team/a",
			pod:         "pod/x",
			container:   "ctr/y",
			wantCluster: "arn:aws:eks:us-east-1:1234_cluster_foo",
			wantName:    "team_a-pod_x-ctr_y-2026-05-18T14-30-45.123.log",
		},
		{
			name:        "dot-dot segments are neutralized",
			cluster:     "..",
			ns:          "..",
			pod:         "x/..",
			container:   "c",
			wantCluster: "__",
			wantName:    "__-x___-c-2026-05-18T14-30-45.123.log",
		},
		{
			name:        "empty cluster falls back via caller (preserved as-is here)",
			cluster:     "",
			ns:          "default",
			pod:         "p",
			container:   "c",
			wantCluster: "",
			wantName:    "default-p-c-2026-05-18T14-30-45.123.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", base)

			got, err := BuildPath(tt.cluster, tt.ns, tt.pod, tt.container, now)
			if err != nil {
				t.Fatalf("BuildPath: %v", err)
			}

			want := filepath.Join(base, "aku", "logs", tt.wantCluster, tt.wantName)
			if got != want {
				t.Errorf("BuildPath\n got:  %s\n want: %s", got, want)
			}
		})
	}
}

func TestBuildPath_HomeDirFallback(t *testing.T) {
	// With XDG_STATE_HOME unset, BuildPath must fall back to ~/.local/state.
	home := t.TempDir()
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", home)

	now := time.Date(2026, 5, 18, 14, 30, 45, 0, time.UTC)
	got, err := BuildPath("prod", "default", "pod", "ctr", now)
	if err != nil {
		t.Fatalf("BuildPath: %v", err)
	}

	want := filepath.Join(home, ".local/state", "aku", "logs", "prod",
		"default-pod-ctr-2026-05-18T14-30-45.000.log")
	if got != want {
		t.Errorf("BuildPath\n got:  %s\n want: %s", got, want)
	}
}

func TestWrite_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	lines := []string{"first", "second", "third"}

	if err := Write(path, lines); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "first\nsecond\nthird\n"
	if string(data) != want {
		t.Errorf("file content\n got:  %q\n want: %q", string(data), want)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file should not exist after success, got err=%v", err)
	}
}

func TestWrite_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")

	if err := os.WriteFile(path, []byte("stale contents that should disappear"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := Write(path, []string{"fresh"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "fresh\n" {
		t.Errorf("file should be replaced, got: %q", string(data))
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file should not exist after success, got err=%v", err)
	}
}

func TestWrite_CreatesNestedDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "out.log")

	if err := Write(path, []string{"hello"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello\n" {
		t.Errorf("file content got: %q", string(data))
	}
}

func TestWrite_CleanupOnWriteFileFailure(t *testing.T) {
	// Skip when the process can bypass DAC permission checks: root on Unix
	// ignores 0o555 dir mode and the WriteFile would succeed anyway.
	if os.Geteuid() == 0 {
		t.Skip("test requires non-root user (read-only dir bypassed by root)")
	}

	// Seed a read-only parent directory so os.WriteFile(tmp, ...) fails with
	// permission denied. MkdirAll on an existing directory is a no-op and
	// preserves the existing mode, so the read-only bit survives the call.
	dir := t.TempDir()
	parent := filepath.Join(dir, "ro")
	if err := os.Mkdir(parent, 0o555); err != nil {
		t.Fatalf("seed read-only dir: %v", err)
	}
	// Restore writable mode so t.TempDir cleanup can remove the tree.
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	path := filepath.Join(parent, "out.log")
	if err := Write(path, []string{"data"}); err == nil {
		t.Fatal("Write should fail when parent dir is read-only")
	}

	// No tmp file should be left behind. We have read+execute on the
	// directory so ReadDir works.
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("stray temp file left after WriteFile failure: %s", e.Name())
		}
	}
}

func TestWrite_CleanupOnRenameFailure(t *testing.T) {
	// Make the destination path a directory so os.Rename(tmp, path) fails:
	// rename of a regular file onto a non-empty directory returns an error on
	// every supported platform. Write must then remove the temp file.
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("seed dir: %v", err)
	}
	// Drop a sentinel inside so the rename target is non-empty (POSIX rename
	// onto an empty dir of a file is also an error, but be defensive).
	if err := os.WriteFile(filepath.Join(path, "marker"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	if err := Write(path, []string{"data"}); err == nil {
		t.Fatal("Write should fail when destination is a directory")
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file should be cleaned up after rename failure, got err=%v", err)
	}
}
