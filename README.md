# aku

<p align="center">
  <img src="media/kubeaku.svg" alt="KubeAku" width="200">
</p>

**A**nother **K**8s **U**I

A terminal UI for managing Kubernetes clusters, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

<p align="center">
  <img src="media/aku.png" alt="aku screenshot" width="800">
</p>

## Features

**Resource browsing**
- Automatic discovery of any CRD or API resource not covered by built-in plugins
- Disambiguation of same-name resources across API groups (e.g. `certificates [cert-manager.io/v1]`)
- Helm release management with values editing, rollback, and chart switching
- Drill-down navigation between related resources (deployment → replicaset → pods → containers)

**Views**
- YAML view with syntax highlighting (managedFields stripped)
- Helm release values view: user-supplied (`v`) or full coalesced (`V`) in the YAML panel
- Describe view with events and environment variable resolution
- Live log streaming with time range presets, container selection, and autoscroll
- Log syntax highlighting (JSON, log levels, IPs, URLs, UUIDs, timestamps, paths, key=value)
- Split panes with independent namespace, filter, and cursor per pane (new splits are inserted adjacent to the focused pane, not appended)
- Vertical and horizontal layout orientation (toggle with `%` or `--layout` flag)
- Zoom to full-screen any split or detail panel

**Operations**
- Edit resources in your `$EDITOR` with automatic retry on validation errors
- Exec into containers as live, embeddable terminal panes (multiple concurrent, zoomable, run in the background)
- Ephemeral debug containers (pods and nodes, with optional privileged mode) as live terminal panes — node-debug pods are cleaned up on pane close and on quit
- Port forwarding with live status tracking
- Update container images across workloads
- Scale deployments, statefulsets, and replicasets
- Rollout restart for deployments, statefulsets, and daemonsets (supports multi-select)
- Multi-select resources for bulk delete and rollout restart
- Helm values editing, rollback to any revision, and chart reference updates
- Save log buffer to file (and optionally open in `$EDITOR`)

**Terminals**
- Exec and debug sessions are first-class panes: place them adjacent to other resources, zoom to focus, keep shells alive in the background
- tmux-style prefix key (default `Ctrl+a`) escapes raw typing: prefix then `h/j/k/l` or arrows moves focus, prefix then `z` zooms, prefix then `x` closes the pane; close a live pane with prefix then `x` (`Ctrl+w` is sent to the shell), while `Ctrl+w` directly closes an *exited* pane like any other
- Send a literal prefix byte to the shell by pressing the prefix twice
- Exited shells keep an `[exited — status N]` banner and scrollback until you close them
- Mouse wheel scrolls a terminal pane's scrollback

**Navigation**
- Vim-style keybindings with multi-key sequences (`gg`, `gp`, `gd`, etc.)
- Fuzzy resource picker (`:`) and namespace picker (`Ctrl+n`)
- Regex search (`/`) and filter (`Ctrl+/`) in both list and detail views
- Column sorting by name, namespace, age, status, or kind
- Multi-cluster context switching: global overlay picker (`gx`), in-pane contexts list (`gX`), or open it in a new split (`oX`) for side-by-side live panes on different clusters
- Fully customizable keybindings via YAML

**Mouse (optional)**
- Wheel scrolls the pane under the cursor without changing focus (over a terminal pane it scrolls that pane's scrollback)
- When an overlay (namespace picker, resource picker, etc.) is open, the wheel scrolls the overlay's list
- Click on a split focuses that split and moves its cursor to the clicked row
- Double-click (two clicks on the same data row within 500 ms (inclusive)) acts as Enter and drills into the resource
- Enable with `mouse.enabled: true` in `config.yaml`

## Installation

### From GitHub releases

Download a prebuilt binary from the [releases page](https://github.com/aohoyd/aku/releases).

### From source

```bash
go install github.com/aohoyd/aku@latest
```

### Build locally

```bash
git clone https://github.com/aohoyd/aku.git
cd aku
make build
```

## Usage

```bash
aku                                    # default kubeconfig and context
aku --context staging                  # specific context
aku -n kube-system                     # specific namespace
aku -r pods,deploy                     # open with specific resources
aku -r pods -d logs                    # open pods with log panel
aku -l horizontal                      # start in horizontal layout
aku -r certificates.cert-manager.io/v1 # qualified resource (when names collide)
aku --kubeconfig /path/to/kubeconfig   # custom kubeconfig path
aku --version                          # show version
```

| Flag | Short | Description |
|------|-------|-------------|
| `--kubeconfig` | | Path to kubeconfig file |
| `--context` | | Kubeconfig context to use |
| `--namespace` | `-n` | Kubernetes namespace |
| `--resource` | `-r` | Resources to display (repeatable) |
| `--details` | `-d` | Open detail panel (`y`/yaml, `d`/describe, `l`/logs) |
| `--layout` | `-l` | Layout orientation (`v`/vertical, `h`/horizontal) |
| `--version` | `-v` | Show version |

aku reads your kubeconfig from `$KUBECONFIG` or `~/.kube/config`.

## Configuration

All configuration files are optional. aku uses sensible defaults when no config files are present.

```
~/.config/aku/
├── config.yaml    # Application settings
├── keymap.yaml    # Custom keybindings
└── theme.yaml     # Color theme
```

The directory follows the XDG Base Directory specification (`$XDG_CONFIG_HOME/aku/`).

### config.yaml

```yaml
# Helm chart references for values editing
charts:
  my-namespace:
    my-release: oci://registry.example.com/charts/my-chart

# Debug container settings
debug:
  image: busybox:latest    # default
  command: ["sh"]          # default

# Log viewer settings
logs:
  buffer_size: 10000       # max lines to buffer (default)

# Extra directories to scan for kubeconfig files (multi-cluster context switching)
contexts:
  directories:
    - ~/.kube/configs       # recursively scanned for kubeconfig files
    - ~/work/clusters       # contexts found here become switchable

# API timeout for async operations (describe, helm, log stream)
api:
  timeout_seconds: 5       # default
  heartbeat_seconds: 5     # cluster health check interval (default)

# Mouse support (off by default — preserves terminal text selection)
mouse:
  enabled: true    # default: false

# Embedded terminal panes (exec / debug sessions)
terminal:
  prefix: ctrl+a   # tmux-style prefix key (default)
  scrollback: 5000 # off-screen lines kept per terminal (default)

# Shell launched by exec (s s) into a container
exec:
  command: ["sh", "-c", "clear; (bash || ash || sh)"]  # default
```

When `mouse.enabled` is `false` or unset, aku does not process mouse events and native terminal text selection works normally.

When mouse support is enabled, aku captures mouse events for click focus, wheel scrolling, and double-click drill-down. Two clicks on the same row within 500 ms (inclusive) are treated as Enter. When no overlay is open, scrolling the wheel over any pane moves that pane's cursor without changing focus. When an overlay is open, the wheel scrolls the overlay's list instead.

To copy text while mouse support is enabled, hold Option (iTerm2, macOS Terminal) or Shift (most Linux terminals) while dragging to bypass aku and use the terminal's native selection.

Exec (`s s`) and debug (`s d` / `s p`) open live terminal panes that run concurrently in the background — split them alongside other resources, zoom to focus, and switch away while a shell stays alive. A focused live terminal owns every keystroke, so navigation goes through the tmux-style prefix (`terminal.prefix`, default `ctrl+a`): press the prefix, then a nav key (`h/j/k/l` or arrows to move focus, `z` to zoom, `x` to close the pane). To close a *live* pane use the prefix then `x` — a bare `Ctrl+w` is forwarded to the shell, not intercepted. Once a pane has exited it behaves like any other split, so `Ctrl+w` closes it directly (no prefix). Press the prefix twice to send a literal prefix byte to the shell. When a shell exits, its pane keeps an `[exited — status N]` banner and scrollback until you close it. If the I/O stream fails mid-session (network drop, API-server error), the pane freezes with a `[exited — status 1] stream error: <detail>` banner instead of a clean status, so a broken connection is never mistaken for a normal exit. If a debug/exec pre-flight fails before the shell ever starts, the placeholder pane is removed (or, when it is the only pane, frozen as a closeable exited pane showing `debug failed: <reason>` that you dismiss like any other exited pane). Under sustained I/O saturation, keystrokes may be dropped rather than blocking — a deliberate tradeoff that keeps the UI responsive when the remote shell can't keep up. Node-debug pods are removed on pane close and on quit; ephemeral debug containers can't be deleted (a Kubernetes limitation) and the pane notes this on exit. `terminal.scrollback` sets how many off-screen lines each terminal retains for wheel/page scrolling. The shell launched by exec (`s s`) is overridable via `exec.command` (default `["sh", "-c", "clear; (bash || ash || sh)"]`, which clears the screen and prefers `bash`, falling back to `ash`/`sh`).

`contexts.directories` lists extra directories aku scans recursively (in addition to the active `$KUBECONFIG`/`~/.kube/config`) for kubeconfig files. Every context across those files becomes switchable; files that aren't valid kubeconfigs or contain zero contexts are skipped. There is no "pinned" pane — every pane simply carries a context, and the focused pane is the source of truth (its context drives new-split defaults and the status bar). `gx` opens a fuzzy overlay picker that moves every pane sharing the focused pane's context to the chosen cluster (focused-context-group move), connecting asynchronously. `gX` opens an in-pane contexts list (a resource view) in the focused pane; Enter switches that pane's context. `oX` opens the contexts list in a new split, so you can bring up another cluster side-by-side. The contexts view has columns NAME, CLUSTER, SERVER, STATUS, and PANES, where STATUS is `●` connected, `○` offline/degraded, or `–` not yet dialed. The `gx` overlay annotates each row: contexts currently open get a `●` marker plus their pane count, and the focused pane's current context is highlighted. All context switches dial asynchronously off the Update goroutine, so the UI never freezes — even when a cluster is unreachable, the pane shows an `⚠ offline` marker instead of hanging. Pane context labels are shown only when more than one distinct context is open across panes; when every pane shares one context, labels are hidden. On switch, a pane lands on the chosen context's default namespace (the kubeconfig `namespace:` field, else `default`); if the pane's current resource type doesn't exist on the new cluster it shows an empty list and a short message in the status bar (there is no inline annotation in the list). That missing-resource check requires the cluster's API discovery to have been populated; until then the pane simply shows an empty list. Different panes can run live on different clusters at the same time, each with its own watches.

### keymap.yaml

```yaml
bindings:
  # Add a custom binding
  - key: "ctrl+l"
    help: "logs"
    command: "view-logs-focused"
    scope: "resources"
    for: ["pods"]
    visible: true

  # Multi-key sequence
  - key: "g"
    scope: "resources"
    keys:
      - key: "i"
        help: "ingresses"
        command: "goto-ingresses"
```

Available scopes: `resources`, `details` (matches all detail views), `yaml`, `describe`, `logs`, `terminal`.

For the full list of command names, see the `defaults.go` keymap source (`internal/config/defaults.go`).

### theme.yaml

```yaml
ui:
  accent: "#7C3AED"
  muted: "#6B7280"
  error: "#EF4444"
  warning: "#F59E0B"

status:
  running: "#10B981"
  failed: "#EF4444"
  pending: "#F59E0B"

syntax:
  key: "#60A5FA"
  string: "#34D399"
  number: "#F472B6"
```

## Key Bindings

### Global

| Key | Action |
|-----|--------|
| `q` | Quit / close overlay |
| `j/k` or `↑/↓` | Cursor up/down (list) / scroll (detail) |
| `h/l` or `←/→` | Scroll left/right |
| `Tab` | Switch panel (resources ↔ details) |
| `Shift+Tab` | Next split pane |
| `Shift+←/→/↑/↓` | Directional focus (orientation-aware) |
| `Alt+Shift+←/→/↑/↓` | Move focused pane (orientation-aware: active axis only; perpendicular direction does nothing) |
| `%` | Toggle layout orientation |
| `Ctrl+n` | Namespace picker |
| `:` | Resource picker |
| `gx` | Move focused pane's context group to another cluster (overlay picker) |
| `gX` | Open contexts list in focused pane |
| `oX` | Open contexts list in a new split |
| `?` | Help overlay |
| `y` | YAML view (resets values variant to manifest for helm releases) |
| `d` | Describe view |
| `e` | Edit resource |
| `Z` | Toggle zoom |
| `Ctrl+r` | Reload all |
| `/` | Search (regex) |
| `\|` / `Ctrl+/` | Filter (regex) |

### Resource List

| Key | Action |
|-----|--------|
| `gg` / `G` | Top / bottom |
| `Enter` | Drill down / open detail |
| `Tab` | Switch to detail panel |
| `Shift+Tab` | Next split pane |
| `Space` | Toggle select |
| `Ctrl+a` | Select all |
| `Ctrl+d` | Delete selected |
| `g + p/d/s/v/c/n` | Go to pods/deployments/secrets/services/configmaps/namespaces |
| `o + p/d/s/v/c` | Open split: pods/deployments/secrets/services/configmaps |
| `gX` / `oX` | Open the contexts list (`g` in current pane, `o` in a new split) |
| `S + n/N/a/s` | Sort by name/namespace/age/status |

### Detail Panel

| Key | Action |
|-----|--------|
| `Tab` | Back to list |
| `w` | Toggle word wrap |
| `x` | Resolve env variables |
| `r` | Refresh |
| `y` | Return to manifest YAML (resets `Values (user)`/`Values (all)` variant on helm releases) |

### Logs View

| Key | Action |
|-----|--------|
| `a` | Toggle autoscroll |
| `s` | Toggle syntax highlighting |
| `c` | Select container |
| `t` | Time range |
| `Enter` | Insert marker |
| `Ctrl+s` | Save log buffer to file |
| `Ctrl+Shift+s` | Save and open in `$EDITOR` |

Saved logs go to `$XDG_STATE_HOME/aku/logs/<cluster>/<ns>-<pod>-<container>-<YYYY-MM-DDTHH-MM-SS.mmm>.log` (default `~/.local/state/aku/logs/...`). The editor used by `Ctrl+Shift+s` is resolved from `$KUBE_EDITOR`, then `$EDITOR`, then `vi`.

`Ctrl+Shift+s` requires a terminal with the Kitty keyboard protocol (Kitty, WezTerm, Ghostty, iTerm2 with the option enabled). On other terminals, remap the `save-and-open-logs` command via `keymap.yaml`.

### Resource-Specific

| Key | Resource | Action |
|-----|----------|--------|
| `l` | Pods, Containers | View logs |
| `s` | Deployments, StatefulSets, ReplicaSets | Scale replicas |
| `i` | Pods, Deployments, StatefulSets, DaemonSets | Set image |
| `R` | Deployments, StatefulSets, DaemonSets | Rollout restart |
| `R` | Helm releases | Rollback |
| `C` | Helm releases | Set chart |
| `v` | Helm releases | View user values |
| `V` | Helm releases | View all (coalesced) values |
| `x` | Pods, Secrets, Containers, Deployments, StatefulSets, DaemonSets | Resolve/show env variables (list and detail) |
| `pf` | Pods, Containers | Port forward |
| `ss` | Pods, Containers | Exec (live terminal pane) |
| `sd` | Pods, Containers, Nodes | Debug container (live terminal pane) |
| `sp` | Pods, Containers, Nodes | Privileged debug (live terminal pane) |

### Terminal Pane

A focused live terminal forwards every keystroke to the shell; use the prefix (default `Ctrl+a`) to navigate.

| Key | Action |
|-----|--------|
| `<prefix>` then `h/j/k/l` or arrows | Move focus |
| `<prefix>` then `z` | Zoom pane |
| `<prefix>` then `x` | Close pane |
| `Ctrl+w` | Close pane (exited pane only; on a live pane it is sent to the shell) |
| `<prefix> <prefix>` | Send a literal prefix byte to the shell |
| `<prefix>` then `PgUp` / `PgDn` | Scroll scrollback (an exited pane also accepts bare `PgUp` / `PgDn`) |

The prefix key is configurable via `terminal.prefix` in `config.yaml`.

## Name

The name is a reference to [Aku](https://en.wikipedia.org/wiki/Aku_(Samurai_Jack)) from Samurai Jack — and also stands for **A**nother **K**8s **U**I.

## License

MIT
