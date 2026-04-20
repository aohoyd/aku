package config

// DefaultBindings returns the default key bindings as a flat list.
func DefaultBindings() []Binding {
	return []Binding{
		// ── Global, visible in statusbar ──
		{Key: "q", Help: "quit/close", Command: "quit", Visible: true},
		{Key: "ctrl+n", Help: "namespace", Command: "namespace-picker", Visible: true},
		{Key: "y", Help: "yaml", Command: "view-yaml-focused", Visible: true},
		{Key: "d", Help: "describe", Command: "view-describe-focused", Visible: true},
		{Key: "x", Help: "uncovered", Command: "view-describe-uncovered", Scope: "resources",
			For: []string{"pods", "secrets", "containers", "deployments", "statefulsets", "daemonsets"}, Visible: true},
		{Key: "Z", Help: "zoom", Command: "toggle-zoom", Visible: true},
		{Key: "e", Help: "edit", Command: "edit", Visible: true},
		{Key: "g", Help: "go to", Visible: true, Keys: []Binding{
			{Key: "g", Help: "top", Command: "cursor-top"},
			{Key: "p", Help: "pods", Command: "goto-pods"},
			{Key: "d", Help: "deployments", Command: "goto-deployments"},
			{Key: "s", Help: "secrets", Command: "goto-secrets"},
			{Key: "v", Help: "services", Command: "goto-services"},
			{Key: "c", Help: "configmaps", Command: "goto-configmaps"},
			{Key: "n", Help: "namespaces", Command: "goto-namespaces"},
			{Key: "S", Help: "statefulsets", Command: "goto-statefulsets"},
			{Key: "D", Help: "daemonsets", Command: "goto-daemonsets"},
			{Key: "f", Help: "port-forwards", Command: "goto-portforwards"},
			{Key: "H", Help: "helm", Command: "goto-helmreleases"},
		}},
		{Key: "G", Help: "bottom", Command: "cursor-bottom"},
		{Key: "o", Help: "split", Visible: true, Keys: []Binding{
			{Key: "p", Help: "pods", Command: "split-pods"},
			{Key: "d", Help: "deployments", Command: "split-deployments"},
			{Key: "s", Help: "secrets", Command: "split-secrets"},
			{Key: "v", Help: "services", Command: "split-services"},
			{Key: "c", Help: "configmaps", Command: "split-configmaps"},
			{Key: "n", Help: "namespaces", Command: "split-namespaces"},
			{Key: "S", Help: "statefulsets", Command: "split-statefulsets"},
			{Key: "D", Help: "daemonsets", Command: "split-daemonsets"},
			{Key: "f", Help: "port-forwards", Command: "split-portforwards"},
			{Key: "H", Help: "helm", Command: "split-helmreleases"},
		}},
		{Key: "ctrl+d", Help: "delete", Command: "delete", Visible: true},
		{Key: "ctrl+f", Help: "page down", Command: "page-down"},
		{Key: "ctrl+b", Help: "page up", Command: "page-up"},
		{Key: "shift+left", Help: "focus left", Command: "focus-left"},
		{Key: "shift+right", Help: "focus right", Command: "focus-right"},
		{Key: "shift+up", Help: "focus up", Command: "focus-up"},
		{Key: "shift+down", Help: "focus down", Command: "focus-down"},
		{Key: "tab", Help: "switch panel", Command: "toggle-panel-focus", Visible: true},
		{Key: "shift+tab", Help: "next split", Command: "focus-next-split"},
		{Key: "%", Help: "layout", Command: "toggle-orientation", Visible: true},
		{Key: "0", Help: "line start", Command: "scroll-home"},
		{Key: "$", Help: "line end", Command: "scroll-end"},

		// ── Global, hidden from statusbar (default) ──
		{Key: ":", Help: "resource picker", Command: "resource-picker"},
		{Key: "esc", Help: "close/clear", Command: "clear-overlay"},
		{Key: "ctrl+w", Help: "close panel", Command: "close-current-panel", Visible: true},
		{Key: "ctrl+r", Help: "reload", Command: "reload-all"},
		{Key: "j", Help: "down", Command: "cursor-down"},
		{Key: "k", Help: "up", Command: "cursor-up"},
		{Key: "h", Help: "scroll left", Command: "scroll-left"},
		{Key: "l", Help: "scroll right", Command: "scroll-right"},
		{Key: "up", Help: "up", Command: "cursor-up"},
		{Key: "down", Help: "down", Command: "cursor-down"},
		{Key: "left", Help: "scroll left", Command: "scroll-left"},
		{Key: "right", Help: "scroll right", Command: "scroll-right"},
		{Key: "n", Help: "next match", Command: "search-next"},
		{Key: "N", Help: "prev match", Command: "search-prev"},
		{Key: "/", Help: "search", Command: "search-open", Visible: true},
		{Key: "ctrl+/", Help: "filter", Command: "filter-open"},
		{Key: "|", Help: "filter", Command: "filter-open", Visible: true},
		{Key: "?", Help: "help", Command: "help"},

		// ── Resources scope ──
		{Key: "enter", Help: "detail", Command: "enter-detail", Scope: "resources"},
		{Key: "space", Help: "select", Command: "toggle-select", Scope: "resources"},
		{Key: "ctrl+a", Help: "select all", Command: "select-all", Scope: "resources"},
		{Key: "pgdown", Help: "page down", Command: "page-down", Scope: "resources"},
		{Key: "pgup", Help: "page up", Command: "page-up", Scope: "resources"},
		{Key: "home", Help: "top", Command: "cursor-top", Scope: "resources"},
		{Key: "end", Help: "bottom", Command: "cursor-bottom", Scope: "resources"},
		{Key: "S", Help: "sort", Scope: "resources", Visible: true, Keys: []Binding{
			{Key: "n", Help: "by name", Command: "sort-NAME"},
			{Key: "N", Help: "by namespace", Command: "sort-NAMESPACE"},
			{Key: "a", Help: "by age", Command: "sort-AGE"},
			{Key: "s", Help: "by status", Command: "sort-STATUS"},
			{Key: "k", Help: "by kind", Command: "sort-KIND", For: []string{"helmmanifest"}},
		}},

		// ── Details scope ──
		{Key: "x", Help: "env resolve", Command: "toggle-env-resolve", Scope: "details",
			For: []string{"pods", "secrets", "containers", "helmmanifest", "deployments", "statefulsets", "daemonsets"}, Visible: true},
		{Key: "w", Help: "wrap", Command: "toggle-wrap", Scope: "details", Visible: true},
		{Key: "alt+e", Help: "header", Command: "toggle-header", Scope: "details", Visible: true},
		{Key: "r", Help: "refresh", Command: "refresh-detail", Scope: "details"},
		{Key: "esc", Help: "close/clear", Command: "clear-overlay", Scope: "details"},
		{Key: "pgdown", Help: "page down", Command: "page-down", Scope: "details"},
		{Key: "pgup", Help: "page up", Command: "page-up", Scope: "details"},
		{Key: "home", Help: "top", Command: "cursor-top", Scope: "details"},
		{Key: "end", Help: "bottom", Command: "cursor-bottom", Scope: "details"},

		// ── Log pager controls ──
		{Key: "a", Help: "autoscroll", Command: "toggle-autoscroll", Scope: "logs", Visible: true},
		{Key: "s", Help: "syntax", Command: "toggle-log-syntax", Scope: "logs", Visible: true},
		{Key: "c", Help: "container", Command: "select-container", Scope: "logs", Visible: true},
		{Key: "t", Help: "time range", Command: "select-time-range", Scope: "logs", Visible: true},
		{Key: "enter", Help: "mark", Command: "log-insert-marker", Scope: "logs"},

		// ── Resource-specific ──
		{Key: "l", Help: "logs", Command: "view-logs-focused",
			For: []string{"pods", "containers"}, Visible: true},
		{Key: "i", Help: "set image", Command: "set-image",
			For: []string{"pods", "containers", "deployments", "statefulsets", "daemonsets"}, Visible: true},
		{Key: "s", Help: "scale", Command: "scale", Scope: "resources",
			For: []string{"deployments", "statefulsets"}, Visible: true},
		{Key: "R", Help: "rollout restart", Command: "rollout-restart",
			For: []string{"pods", "deployments"}, Visible: true},
		{Key: "R", Help: "rollback", Command: "helm-rollback",
			For: []string{"helmreleases"}, Visible: true},
		{Key: "C", Help: "set chart", Command: "helm-set-chart",
			For: []string{"helmreleases"}, Visible: true},
		{Key: "s", Help: "exec", Scope: "resources", Visible: true,
			For: []string{"pods", "containers", "nodes"}, Keys: []Binding{
				{Key: "d", Help: "debug", Command: "debug"},
				{Key: "p", Help: "debug privileged", Command: "debug-privileged"},
				{Key: "s", Help: "exec", Command: "exec",
					For: []string{"pods", "containers"}, Visible: true},
			}},
		{Key: "f", Help: "port-forward", Command: "port-forward", For: []string{"pods", "containers"}},
		{Key: "F", Help: "port-forwards", Scope: "resources", Command: "split-portforwards",
			For: []string{"pods", "containers"}, Visible: true},
	}
}

// DefaultKeyTrie returns a KeyTrie with default bindings for the most common
// context (global + pods merged). Used by existing tests.
func DefaultKeyTrie() *KeyTrie {
	bs := NewBindingSet(DefaultBindings())
	return bs.TrieFor("resources", "pods")
}
