package logs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BuildPath returns the absolute path where logs for the given identity should
// be saved at time now. Each identity segment is sanitized so that path
// separators inside names cannot escape the target directory.
func BuildPath(cluster, ns, pod, container string, now time.Time) (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		base = filepath.Join(home, ".local/state")
	}

	cluster = sanitize(cluster)
	ns = sanitize(ns)
	pod = sanitize(pod)
	container = sanitize(container)
	// "-" instead of ":" so the file name is safe on every common file system
	// (Windows, exFAT, SMB shares). Milliseconds prevent rapid Ctrl+S presses
	// from silently overwriting the previous save.
	ts := now.UTC().Format("2006-01-02T15-04-05.000")

	name := fmt.Sprintf("%s-%s-%s-%s.log", ns, pod, container, ts)
	return filepath.Join(base, "aku", "logs", cluster, name), nil
}

// Write atomically writes lines to path, creating parent directories as
// needed. Each line is followed by a newline. On failure, any temporary file
// is removed and the underlying error is returned wrapped.
func Write(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	var b bytes.Buffer
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b.Bytes(), 0o644); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write temp log file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("publish log file: %w", err)
	}
	return nil
}

// sanitize neutralizes path separators and ".." sequences so a user-controlled
// segment (e.g. a kube context name) cannot escape the target directory. Other
// shell-special characters are left intact: on Linux/macOS only "/" and NUL
// are illegal in filenames, and aku targets POSIX file systems.
func sanitize(seg string) string {
	seg = strings.ReplaceAll(seg, "/", "_")
	seg = strings.ReplaceAll(seg, "..", "__")
	return seg
}
