package config

// DefaultBindings returns the default key bindings as a flat list.
func DefaultBindings() []Binding {
	return []Binding{
		// ── Global, visible in statusbar ──
		{Key: "q", Help: "quit/close", Command: "quit", Visible: true},
		{Key: "ctrl+n", Help: "namespace", Command: "namespace-picker", Visible: true},
		{Key: "y", Help: "yaml", Command: "view-yaml-focused", Visible: true},
		{Key: "d", Help: "describe", Command: "view-describe-focused", Visible: true},
		{Key: "Z", Help: "zoom", Command: "toggle-zoom", Visible: true},
		{Key: "v", Help: "view", Visible: true, Keys: []Binding{
			{Key: "y", Help: "yaml", Command: "view-yaml"},
			{Key: "d", Help: "describe", Command: "view-describe"},
		}},
		{Key: "e", Help: "edit", Command: "edit", Visible: true},

		// ── Global, hidden from statusbar (default) ──
		{Key: ":", Help: "resource picker", Command: "resource-picker"},
		{Key: "esc", Help: "close/clear", Command: "clear-overlay"},
		{Key: "ctrl+r", Help: "reload", Command: "reload-all"},
		{Key: "j", Help: "down", Command: "cursor-down"},
		{Key: "k", Help: "up", Command: "cursor-up"},
		{Key: "up", Help: "up", Command: "cursor-up"},
		{Key: "down", Help: "down", Command: "cursor-down"},
		{Key: "n", Help: "next match", Command: "search-next"},
		{Key: "N", Help: "prev match", Command: "search-prev"},
		{Key: "/", Help: "search", Command: "search-open", Visible: true},
		{Key: "ctrl+/", Help: "filter", Command: "filter-open"},
		{Key: "|", Help: "filter", Command: "filter-open", Visible: true},
		{Key: "?", Help: "help", Command: "help"},

		// ── Resource-list scope ──
		{Key: "enter", Help: "detail", Command: "enter-detail", Scope: "resource-list"},
		{Key: "right", Help: "detail", Command: "focus-panel", Scope: "resource-list"},
		{Key: "tab", Help: "next split", Command: "focus-next", Scope: "resource-list", Visible: true},
		{Key: "shift+tab", Help: "prev split", Command: "focus-prev", Scope: "resource-list", Visible: true},
		{Key: "ctrl+f", Help: "page down", Command: "page-down", Scope: "resource-list", Visible: true},
		{Key: "ctrl+b", Help: "page up", Command: "page-up", Scope: "resource-list", Visible: true},
		{Key: "G", Help: "bottom", Command: "cursor-bottom", Scope: "resource-list"},
		{Key: "ctrl+d", Help: "delete", Command: "delete", Scope: "resource-list", Visible: true},
		{Key: "space", Help: "select", Command: "toggle-select", Scope: "resource-list"},
		{Key: "ctrl+a", Help: "select all", Command: "select-all", Scope: "resource-list"},
		{Key: "shift+left", Help: "scroll left", Command: "list-scroll-left", Scope: "resource-list"},
		{Key: "shift+right", Help: "scroll right", Command: "list-scroll-right", Scope: "resource-list"},
		{Key: "pgdown", Help: "page down", Command: "page-down", Scope: "resource-list"},
		{Key: "pgup", Help: "page up", Command: "page-up", Scope: "resource-list"},
		{Key: "home", Help: "top", Command: "cursor-top", Scope: "resource-list"},
		{Key: "end", Help: "bottom", Command: "cursor-bottom", Scope: "resource-list"},
		{Key: "g", Help: "go to", Scope: "resource-list", Visible: true, Keys: []Binding{
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
		{Key: "h", Help: "split", Scope: "resource-list", Visible: true, Keys: []Binding{
			{Key: "p", Help: "pods", Command: "split-pods"},
			{Key: "d", Help: "deployments", Command: "split-deployments"},
			{Key: "s", Help: "secrets", Command: "split-secrets"},
			{Key: "v", Help: "services", Command: "split-services"},
			{Key: "c", Help: "configmaps", Command: "split-configmaps"},
			{Key: "n", Help: "namespaces", Command: "split-namespaces"},
			{Key: "S", Help: "statefulsets", Command: "split-statefulsets"},
			{Key: "D", Help: "daemonsets", Command: "split-daemonsets"},
			{Key: "H", Help: "helm", Command: "split-helmreleases"},
		}},
		{Key: "S", Help: "sort", Scope: "resource-list", Visible: true, Keys: []Binding{
			{Key: "n", Help: "by name", Command: "sort-NAME"},
			{Key: "N", Help: "by namespace", Command: "sort-NAMESPACE"},
			{Key: "a", Help: "by age", Command: "sort-AGE"},
			{Key: "s", Help: "by status", Command: "sort-STATUS"},
			{Key: "k", Help: "by kind", Command: "sort-KIND", For: []string{"helmmanifest"}},
		}},

		// ── Detail-panel scope ──
		{Key: "x", Help: "env resolve", Command: "toggle-env-resolve", Scope: "detail-panel",
			For: []string{"pods", "secrets", "containers", "helmmanifest"}, Visible: true},
		{Key: "H", Help: "scroll left", Command: "scroll-left", Scope: "detail-panel"},
		{Key: "L", Help: "scroll right", Command: "scroll-right", Scope: "detail-panel"},
		{Key: "shift+left", Help: "scroll left", Command: "scroll-left", Scope: "detail-panel"},
		{Key: "shift+right", Help: "scroll right", Command: "scroll-right", Scope: "detail-panel"},
		{Key: "ctrl+f", Help: "page down", Command: "page-down", Scope: "detail-panel"},
		{Key: "ctrl+b", Help: "page up", Command: "page-up", Scope: "detail-panel"},
		{Key: "g", Help: "go to", Scope: "detail-panel", Keys: []Binding{
			{Key: "g", Help: "top", Command: "cursor-top"},
		}},
		{Key: "G", Help: "bottom", Command: "cursor-bottom", Scope: "detail-panel"},
		{Key: "w", Help: "wrap", Command: "toggle-wrap", Scope: "detail-panel", Visible: true},
		{Key: "r", Help: "refresh", Command: "refresh-detail", Scope: "detail-panel"},
		{Key: "left", Help: "back", Command: "exit-detail", Scope: "detail-panel"},
		{Key: "h", Help: "back", Command: "exit-detail", Scope: "detail-panel"},
		{Key: "esc", Help: "close/clear", Command: "clear-overlay", Scope: "detail-panel"},
		{Key: "pgdown", Help: "page down", Command: "page-down", Scope: "detail-panel"},
		{Key: "pgup", Help: "page up", Command: "page-up", Scope: "detail-panel"},
		{Key: "home", Help: "top", Command: "cursor-top", Scope: "detail-panel"},
		{Key: "end", Help: "bottom", Command: "cursor-bottom", Scope: "detail-panel"},

		// ── Log pager controls (detail-panel scope, pods/containers only) ──
		{Key: "a", Help: "autoscroll", Command: "toggle-autoscroll", Scope: "detail-panel",
			For: []string{"pods", "containers"}, Visible: true},
		{Key: "c", Help: "container", Command: "select-container", Scope: "detail-panel",
			For: []string{"pods", "containers"}, Visible: true},
		{Key: "t", Help: "time range", Command: "select-time-range", Scope: "detail-panel",
			For: []string{"pods", "containers"}, Visible: true},

		// ── Resource-specific ──
		{Key: "l", Help: "logs", Command: "view-logs-focused",
			For: []string{"pods", "containers"}, Visible: true},
		{Key: "i", Help: "set image", Command: "set-image",
			For: []string{"pods", "containers", "deployments", "statefulsets", "daemonsets"}, Visible: true},
		{Key: "R", Help: "rollout restart", Command: "rollout-restart",
			For: []string{"pods", "deployments"}, Visible: true},
		{Key: "R", Help: "rollback", Command: "helm-rollback",
			For: []string{"helmreleases"}, Visible: true},
		{Key: "C", Help: "set chart", Command: "helm-set-chart",
			For: []string{"helmreleases"}, Visible: true},
		{Key: "p", Help: "...", For: []string{"pods", "containers"}, Keys: []Binding{
			{Key: "f", Help: "port-forward", Command: "port-forward"},
		}},
		{Key: "v", Help: "view", Visible: true, For: []string{"pods", "containers"}, Keys: []Binding{
			{Key: "y", Help: "yaml", Command: "view-yaml"},
			{Key: "d", Help: "describe", Command: "view-describe"},
			{Key: "l", Help: "logs", Command: "view-logs"},
		}},
		{Key: "s", Help: "exec", Scope: "resource-list", Visible: true,
			For: []string{"pods", "containers", "nodes"}, Keys: []Binding{
				{Key: "d", Help: "debug", Command: "debug"},
				{Key: "p", Help: "debug privileged", Command: "debug-privileged"},
				{Key: "s", Help: "exec", Command: "exec",
					For: []string{"pods", "containers"}, Visible: true},
			}},
	}
}

// DefaultKeyTrie returns a KeyTrie with default bindings for the most common
// context (global + pods merged). Used by existing tests.
func DefaultKeyTrie() *KeyTrie {
	bs := NewBindingSet(DefaultBindings())
	return bs.TrieFor("resource-list", "pods")
}
