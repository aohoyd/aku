package cluster

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"k8s.io/client-go/tools/clientcmd"
)

// ScanKubeconfigs discovers kube-contexts by parsing kubeconfig files. It
// mirrors the user's fish `ks` function: the default kubeconfig is processed
// first so its context names win on collision, then each directory is walked
// recursively and every readable file is tried as a kubeconfig.
//
// Files that fail to parse as kubeconfig YAML or that contain zero contexts are
// skipped silently (this is how non-kubeconfig YAML is filtered out, mirroring
// the `select(.contexts | length != 0)` in the fish function). Duplicate
// context names are deduped first-writer-wins; because defaultPath is processed
// before the directories, default contexts own their names on collision.
//
// Missing or unreadable directories and files are tolerated: the scan skips
// them and continues rather than aborting. The returned slice is sorted by Name
// for a deterministic flat picker list.
//
// defaultPaths are the canonical kubeconfig files (see DefaultKubeconfigFiles):
// the --kubeconfig flag, the $KUBECONFIG entries, and ~/.kube/config. They are
// processed first, in order, so their contexts win on a name collision with a
// directory file (and earlier default paths win over later ones).
//
// There is no error return: every failure mode (unparseable kubeconfig,
// zero-context file, missing/unreadable dir or file) is intentionally skipped
// silently, so the scan can never fail as a whole. The previous error return was
// always nil; callers that discarded it no longer need to.
func ScanKubeconfigs(dirs []string, defaultPaths []string) []ContextEntry {
	var entries []ContextEntry
	seen := make(map[string]bool)

	// addFile parses path as a kubeconfig and records its (new) contexts.
	// Parse failures and zero-context files are skipped silently.
	addFile := func(path string) {
		cfg, err := clientcmd.LoadFromFile(path)
		if err != nil || cfg == nil || len(cfg.Contexts) == 0 {
			return
		}
		for name := range cfg.Contexts {
			if seen[name] {
				continue
			}
			seen[name] = true
			entries = append(entries, ContextEntry{Name: name, File: path})
		}
	}

	// Process the default kubeconfig files first so their contexts win on
	// collision (in order, so earlier paths win over later ones).
	for _, p := range defaultPaths {
		p = ExpandPath(p)
		if p == "" {
			continue
		}
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			addFile(p)
		}
	}

	// Walk each configured directory recursively.
	for _, dir := range dirs {
		// Expand `~` and environment variables so config values like
		// `~/.kube/configs` or `$HOME/clusters` resolve as users expect.
		dir = ExpandPath(dir)
		if dir == "" {
			continue
		}
		// A missing dir is skipped gracefully; only walk if it exists.
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			// Tolerate per-entry errors (e.g. unreadable subdir): skip and
			// continue rather than aborting the whole walk.
			if err != nil {
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() || !d.Type().IsRegular() {
				return nil
			}
			addFile(path)
			return nil
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// CurrentContextName reads the kubeconfig at path and returns its
// current-context name, or "" if the file cannot be parsed or names no
// current-context. It exists so a degraded startup (where the cluster failed to
// connect and carries an empty context) can still resolve a meaningful context
// name for seeding initial panes, instead of stamping them with "".
func CurrentContextName(path string) string {
	if path == "" {
		return ""
	}
	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil || cfg == nil {
		return ""
	}
	return cfg.CurrentContext
}
