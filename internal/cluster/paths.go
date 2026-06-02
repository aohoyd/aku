package cluster

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath resolves environment variables and a leading `~` in a filesystem
// path, so config values like `~/.kube/configs` or `$HOME/clusters` work as
// users expect. Environment variables are expanded first (`$VAR` / `${VAR}`),
// then a leading `~` or `~/` is replaced with the user's home directory.
//
// Only the current user's home is expanded: the `~username` form (another
// user's home) is NOT supported and is returned unchanged. It is best-effort and
// never fails: if the home directory cannot be resolved, the `~` form is
// returned unchanged. Relative paths are left relative (no filepath.Abs),
// matching the prior behavior for non-`~` entries.
func ExpandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = os.ExpandEnv(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		if p == "~" {
			return home
		}
		return filepath.Join(home, p[2:])
	}
	return p
}

// DefaultKubeconfigFiles returns the kubeconfig files aku scans for contexts:
// the --kubeconfig flag (if set), then each entry of $KUBECONFIG (split on the
// OS path-list separator, since KUBECONFIG is a list), then the canonical
// ~/.kube/config — always included last so a context there (e.g. the
// current-context) shows up even when $KUBECONFIG points elsewhere.
//
// Each entry is ~/env-expanded via ExpandPath; empties are skipped and
// duplicates are dropped by cleaned path (first occurrence wins, preserving the
// flag > $KUBECONFIG > default precedence for which file owns a context on a
// later name collision in ScanKubeconfigs).
func DefaultKubeconfigFiles(flag string) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(list string) {
		for _, p := range filepath.SplitList(list) {
			p = ExpandPath(p)
			if p == "" {
				continue
			}
			key := filepath.Clean(p)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, p)
		}
	}
	add(flag)
	add(os.Getenv("KUBECONFIG"))
	add("~/.kube/config")
	return out
}
