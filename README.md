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
- Zoom the focused pane to fullscreen with `alt+z` (resource list, terminal, or detail/log panel) — a single top bar, status bar hidden; `alt+z` again to exit

**Manifest visualization**
- Pipe rendered manifests into aku (`helm template ./chart | aku`, `kustomize build ./overlay | aku`) or open files/dirs with `-f` and browse them as a simulated cluster — controllers' runtime is fabricated (Deployment→ReplicaSet→Pods, StatefulSet/DaemonSet→Pods, Job/CronJob→Pods, Service→Endpoints) so drill-down, health, and resource lists all work, with `# Source:` provenance surfaced in the YAML view

**Operations**
- Edit resources in your `$EDITOR` with automatic retry on validation errors
- Exec into containers as live, embeddable terminal panes (multiple concurrent, zoomable, run in the background)
- Ephemeral debug containers (pods and nodes, with optional privileged mode) as live terminal panes — node-debug pods are cleaned up on pane close and on quit
- Port forwarding with live status tracking
- Update container images and per-container `imagePullPolicy` across workloads
- Scale deployments, statefulsets, and replicasets
- Rollout restart for deployments, statefulsets, and daemonsets (supports multi-select)
- Multi-select resources for bulk delete and rollout restart
- Helm values editing, rollback to any revision, and chart reference updates
- Save log buffer to file (and optionally open in `$EDITOR`)
- Copy resource name/YAML or open any pane's buffer in your editor (`c` chord)

**Notifications**
- noice.nvim-style toast overlay for aku's own info/warning/error messages — floats top-right (newest on top), never steals focus, auto-hides per level, with a `+N more…` line past a configurable cap (`ctrl+x` clears live toasts)
- `aku-messages` resource (`g m`, short name `msg`) — the full session-wide message history, browsable like Events, with an `Enter` detail view for the untruncated message

**Terminals**
- Exec and debug sessions are first-class panes: place them adjacent to other resources, zoom to focus, keep shells alive in the background
- tmux-style prefix key (default `Ctrl+a`) escapes raw typing: prefix then `h/j/k/l` or arrows moves focus, prefix then `x`/`q`/`Ctrl+w` closes the pane; close a live pane with the prefix then `x`/`q`/`Ctrl+w` (a *bare* `Ctrl+w` is sent to the shell), while a bare `Ctrl+w` directly closes an *exited* pane like any other
- `alt+z` zooms the focused terminal to fullscreen even over a live shell — its top bar shows an `alt+z: exit zoom` hint; `alt+z` and `shift+arrows` are captured by aku (kept out of the shell), every other key goes to the shell
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

helm template ./chart | aku            # browse rendered manifests as a fake cluster
kustomize build ./overlay | aku        # same, via kustomize
aku -f ./manifests/                    # read manifest files/dirs instead of a pipe
```

| Flag | Short | Description |
|------|-------|-------------|
| `--kubeconfig` | | Path to kubeconfig file |
| `--context` | | Kubeconfig context to use |
| `--namespace` | `-n` | Kubernetes namespace |
| `--resource` | `-r` | Resources to display (repeatable) |
| `--file` | `-f` | Manifest file or directory to visualize (repeatable; dirs scanned for `*.yaml`/`*.yml`) |
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
├── theme.yaml     # Color theme (override layer)
└── themes/        # Named theme presets (selected via `theme:` in config.yaml)
    └── midnight-commander.yaml   # example — copy from repo themes/ (aku never writes it)
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

# Notification toasts and the aku-messages history
notifications:
  buffer_size: 1000        # message history ring-buffer size (default)
  max_visible: 5           # toasts shown before "+N more…" (default)
  timeout_info: 3          # seconds before info toasts auto-hide (default)
  timeout_warning: 5       # seconds before warning toasts auto-hide (default)
  timeout_error: 8         # seconds before error toasts auto-hide (default)
  # timeout_*: 0 (or omitted) uses the default; a negative value is sticky
  # (never auto-hides), e.g. timeout_error: -1 keeps errors until dismissed.

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

# Named color theme — loads ~/.config/aku/themes/<name>.yaml
theme: midnight-commander
```

`theme` selects a named theme preset from `~/.config/aku/themes/<name>.yaml`. Themes layer in order: built-in defaults → the named theme file → `theme.yaml` on top (so `theme.yaml` stays a fine-tune override layer, and both are fully optional). If the named theme has no matching file (or a theme file fails to parse), aku prints a warning to stderr and skips that layer — the built-in defaults and any `theme.yaml` still apply on top. It's non-fatal. Only the first warning is shown: if both the named theme is missing/invalid *and* `theme.yaml` fails to parse, only the named-theme warning is reported (the `theme.yaml` parse error is not separately surfaced).

When `mouse.enabled` is `false` or unset, aku does not process mouse events and native terminal text selection works normally.

When mouse support is enabled, aku captures mouse events for click focus, wheel scrolling, and double-click drill-down. Two clicks on the same row within 500 ms (inclusive) are treated as Enter. When no overlay is open, scrolling the wheel over any pane moves that pane's cursor without changing focus. When an overlay is open, the wheel scrolls the overlay's list instead.

To copy text while mouse support is enabled, hold Option (iTerm2, macOS Terminal) or Shift (most Linux terminals) while dragging to bypass aku and use the terminal's native selection.

Exec (`s s`) and debug (`s d` / `s p`) open live terminal panes that run concurrently in the background — split them alongside other resources, zoom to focus, and switch away while a shell stays alive. A focused live terminal owns most keystrokes, so navigation goes through the tmux-style prefix (`terminal.prefix`, default `ctrl+a`): press the prefix, then a nav key (`h/j/k/l` or arrows to move focus, `x`/`q`/`Ctrl+w` to close the pane). Zoom and focus movement are an exception: by default `alt+z` (zoom) and `shift+left/right/up/down` (move focus between panes) are *captured* — handled by aku rather than forwarded to the shell — so you can zoom or switch panes without leaving the terminal (see the `capture:` keymap field below). `alt+z` is a unified fullscreen zoom: the focused pane (resource list, terminal, or detail/log panel) fills the whole screen borderlessly with a single top bar and the status bar hidden; press `alt+z` again to exit. Only one pane is zoomed at a time, but while zoomed you can still move focus with `shift+arrows` — zoom follows the newly-focused split. A zoomed terminal's top bar shows an `alt+z: exit zoom` hint. To close a *live* pane use the prefix then `x`, `q`, or `Ctrl+w` — a *bare* `Ctrl+w` (no prefix) is forwarded to the shell, not intercepted. Once a pane has exited it behaves like any other split, so `Ctrl+w` closes it directly (no prefix). Press the prefix twice to send a literal prefix byte to the shell. When a shell exits, its pane keeps an `[exited — status N]` banner and scrollback until you close it. If the I/O stream fails mid-session (network drop, API-server error), the pane freezes with a `[exited — status 1] stream error: <detail>` banner instead of a clean status, so a broken connection is never mistaken for a normal exit. If a debug/exec pre-flight fails before the shell ever starts, the placeholder pane is removed (or, when it is the only pane, frozen as a closeable exited pane showing `debug failed: <reason>` that you dismiss like any other exited pane). Under sustained I/O saturation, keystrokes may be dropped rather than blocking — a deliberate tradeoff that keeps the UI responsive when the remote shell can't keep up. Node-debug pods are removed on pane close and on quit; ephemeral debug containers can't be deleted (a Kubernetes limitation) and the pane notes this on exit. `terminal.scrollback` sets how many off-screen lines each terminal retains for wheel/page scrolling. The shell launched by exec (`s s`) is overridable via `exec.command` (default `["sh", "-c", "clear; (bash || ash || sh)"]`, which clears the screen and prefers `bash`, falling back to `ash`/`sh`).

`contexts.directories` lists extra directories aku scans recursively (in addition to the active `$KUBECONFIG`/`~/.kube/config`) for kubeconfig files. Every context across those files becomes switchable; files that aren't valid kubeconfigs or contain zero contexts are skipped. There is no "pinned" pane — every pane simply carries a context, and the focused pane is the source of truth (its context drives new-split defaults and the status bar). `gx` opens a fuzzy overlay picker that moves every pane sharing the focused pane's context to the chosen cluster (focused-context-group move), connecting asynchronously. `gX` opens an in-pane contexts list (a resource view) in the focused pane; Enter switches that pane's context. `oX` opens the contexts list in a new split, so you can bring up another cluster side-by-side. The contexts view has columns NAME, CLUSTER, SERVER, STATUS, and PANES, where STATUS is `●` in use (green when connected, red when offline/degraded), `○` idle (no panes), or `◆` a pinned pseudo-context (the static `manifests` cluster). The `gx` overlay annotates each row: contexts currently open get a `●` marker plus their pane count, and the focused pane's current context is highlighted. All context switches dial asynchronously off the Update goroutine, so the UI never freezes — even when a cluster is unreachable, the pane shows an `⚠ offline` marker instead of hanging. Pane context labels are shown only when more than one distinct context is open across panes; when every pane shares one context, labels are hidden. On switch, a pane lands on the chosen context's default namespace (the kubeconfig `namespace:` field, else `default`); if the pane's current resource type doesn't exist on the new cluster it shows an empty list and a short message in the status bar (there is no inline annotation in the list). That missing-resource check requires the cluster's API discovery to have been populated; until then the pane simply shows an empty list. Different panes can run live on different clusters at the same time, each with its own watches.

aku surfaces its own info/warning/error messages as noice.nvim-style **toasts** that float in the top-right corner (newest on top), non-modally — they never steal focus from the pane you're working in. Each toast auto-hides after a per-level timeout: `timeout_info` (default `3`s), `timeout_warning` (default `5`s), and `timeout_error` (default `8`s), so errors linger longer than transient successes. A timeout of `0` (or omitted) falls back to the default; a *negative* value makes that level **sticky** — it never auto-hides until dismissed (e.g. `timeout_error: -1` keeps every error on screen until you clear it). At most `max_visible` toasts (default `5`) are shown at once; any beyond that collapse into a `+N more…` line. Press `ctrl+x` to clear all live toasts at once (the history is kept). Async operation results — scale, delete, restart, helm, context switch, port-forward, save-logs, and the like — now appear as toasts rather than in the status bar, so the status bar shows only key hints and the activity spinner. The full session-wide history lives in the **`aku-messages`** resource (open it with `g m`, or via the `:` picker / short name `msg`), browsable like Events with columns TIME / LEVEL / CONTEXT / SOURCE / MESSAGE, newest-first. The TIME column shows a *relative age* (e.g. `2m`, `1h`) — how long ago the message was recorded, just like the Events view's LAST SEEN column — not a wall-clock time. Press `Enter` on a row for a detail view that shows, in order, the full untruncated message body first, then its level, the exact timestamp, the origin context, and the source. `notifications.buffer_size` (default `1000`) caps that history ring buffer.

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

  # Stay in aku even over a focused live terminal
  - key: "alt+z"
    help: "zoom"
    command: "toggle-zoom"
    capture: true

  # Multi-key sequence
  - key: "g"
    scope: "resources"
    keys:
      - key: "i"
        help: "ingresses"
        command: "goto-ingresses"
```

Available scopes: `resources`, `details` (matches all detail views), `yaml`, `describe`, `logs`, `terminal`.

`capture: true` marks a single-press binding as captured: aku handles the key itself even while a live terminal pane is focused, instead of forwarding it to the shell. (Capture applies only to top-level single-press bindings, not multi-key chord children.) By default the captured keys are `alt+z` (zoom) and `shift+left/right/up/down` (move focus between panes); every other key — including plain arrows and `shift+tab` — is sent to the shell when a live terminal is focused, and the `ctrl+a` prefix still works as before.

For the full list of command names, see the `defaults.go` keymap source (`internal/config/defaults.go`).

### theme.yaml

```yaml
ui:
  # background and foreground are unset by default. Uncomment to paint a
  # full-screen canvas instead of using your terminal's own colors.
  # background: "#1a1a2e"
  # foreground: "#e0e0e0"
  accent: "#7C3AED"
  muted: "#6B7280"
  error: "#EF4444"
  warning: "#F59E0B"
  # text_on_status: "#1F1F28"  # dark text on red/yellow status-colored cursor rows

status:
  running: "#10B981"
  failed: "#EF4444"
  pending: "#F59E0B"

syntax:
  key: "#60A5FA"
  string: "#34D399"
  number: "#F472B6"
```

The example above is partial — it shows a representative subset of keys. The full set of settable keys (all `ui`, `status`, `syntax`, `search`, and `log` fields) is enumerated in the theme source (`internal/theme/theme.go`); any key you omit keeps its default.

The `status.failed` and `status.pending` colors also drive k9s-style whole-row health tinting across pods, containers, deployments, statefulsets, daemonsets, and replicasets. What tints red (`status.failed`) versus yellow (`status.pending`) depends on the resource kind:

- **Pods** classify by pod phase: failure phases (`Failed`, `OOMKilled`, `Terminating`, and prefixes like `Init:*`, `Signal:*`, `ExitCode:*`, plus `CrashLoopBackOff`/`ImagePullBackOff`) tint red; transitional phases (`Pending`, `ContainerCreating`) tint yellow.
- **Containers** classify by *container state*, not pod phase: a waiting container with a known failure reason (`ErrImagePull`, `ImagePullBackOff`, `CrashLoopBackOff`, `CreateContainerError`, `CreateContainerConfigError`, `InvalidImageName`, `RunContainerError`), a terminated container with a non-zero exit code, or a running-but-not-ready container tints red; any other waiting state (e.g. `ContainerCreating`) tints yellow.
- **Deployments** tint red on an explicit failure condition — `Available=False` *or* a `Progressing` condition with reason `ProgressDeadlineExceeded` — and otherwise yellow while ready replicas are below desired.
- **StatefulSets, DaemonSets, ReplicaSets** only ever tint yellow (ready < desired) and never red. Among workloads, only Deployments can go red.

Healthy, fully-ready rows stay default-colored. **Marks always win**: a marked row keeps its mark style and a marked cursor row uses a combined mark+cursor style. For unmarked rows the rule depends on whether the row is under the cursor and whether the pane's selection is active — that is, whether the pane's list *or* its detail panel is focused. Non-cursor unready/transitional rows get the foreground health tint (red/yellow). The unmarked cursor row depends on that selection state: while the pane's selection is active (its list or detail panel is focused) an unready/transitional cursor row is *filled* with the status color (red/yellow) and rendered with dark text (`ui.text_on_status`) — health overrides the plain selection style for that one row so the k9s-style signal stays visible under the cursor, including while you scroll the detail panel — while a healthy cursor row keeps the plain selection style; in an **inactive** pane (another pane is active — a full blur, not merely a dimmed border) the cursor row is rendered exactly like a normal row — the health tint (red/yellow) if it's unready, plain if it's healthy — with no cursor highlight.

`ui.background` and `ui.foreground` set the global terminal background and foreground. Both are unset by default; they're used by full-canvas themes (like Midnight Commander) that paint the whole screen rather than relying on your terminal's own colors.

To install the bundled Midnight Commander theme, create the themes directory and copy `themes/midnight-commander.yaml` from the repo into it (`mkdir -p ~/.config/aku/themes/ && cp themes/midnight-commander.yaml ~/.config/aku/themes/`), then set `theme: midnight-commander` in `config.yaml`. It gives aku a classic blue MC-style canvas.

To install the bundled Kaku Dark theme, copy `themes/kaku-dark.yaml` the same way (`mkdir -p ~/.config/aku/themes/ && cp themes/kaku-dark.yaml ~/.config/aku/themes/`), then set `theme: kaku-dark` in `config.yaml`. It paints a deep `#15141B` canvas with a purple cursor accent and sand-orange highlights, modeled on the [Kaku terminal](https://github.com/tw93/Kaku).

## Embedded terminals

aku runs exec and debug sessions as **live, first-class panes** — they sit alongside your resource lists, keep running in the background while you work elsewhere, and can be zoomed to fullscreen.

### Opening a terminal

| Keys | From | Opens |
|------|------|-------|
| `s s` | Pods, Containers | Exec shell into the container |
| `s d` | Pods, Containers, Nodes | Ephemeral debug container |
| `s p` | Pods, Containers, Nodes | Privileged debug container |

Each session opens as a new split next to the focused pane. Open as many as you like — they run concurrently, and switching focus away leaves the shell alive.

### Typing vs. navigating

A focused *live* terminal forwards almost every keystroke straight to the shell, so aku's normal keybindings don't fire there. Two ways to drive aku from inside a terminal:

- **Captured keys** (handled by aku, never sent to the shell): `alt+z` to zoom and `shift+←/→/↑/↓` to move focus between panes. These work without leaving the shell.
- **The prefix key** (tmux-style, default `ctrl+a`): press the prefix, then a command key:
  - `h/j/k/l` or arrows — move focus
  - `x`, `q`, or `Ctrl+w` — close the pane (a *bare* `Ctrl+w` without the prefix is still forwarded to the shell)
  - `PgUp` / `PgDn` — scroll the scrollback
  - press the prefix **twice** to send a literal prefix byte to the shell

### Zooming

`alt+z` toggles a unified fullscreen zoom: the focused pane fills the screen borderlessly with a single top bar and the status bar hidden. It behaves the same for terminals, resource lists, and the detail/log panel. While zoomed you can still `shift+arrow` to another split — zoom follows focus. A zoomed terminal's top bar shows an `alt+z: exit zoom` hint.

### When a shell ends

- A clean exit freezes the pane with an `[exited — status N]` banner; its scrollback stays readable until you close it. An exited pane behaves like a normal split, so `ctrl+w` (no prefix) closes it.
- A mid-session I/O failure (network drop, API-server error) freezes it with `[exited — status 1] stream error: <detail>`, so a broken connection is never mistaken for a normal exit.
- If a debug/exec pre-flight fails before the shell starts, the placeholder pane is removed — or, when it was the only pane, frozen as a closeable pane showing `debug failed: <reason>`.

Node-debug pods are deleted on pane close and on quit. Ephemeral debug containers can't be deleted (a Kubernetes limitation); the pane notes this on exit.

Closing a pane (or quitting aku) best-effort terminates the remote shell inside the pod rather than leaving it orphaned. This applies to exec, ephemeral-debug, and node-debug sessions alike — for plain exec sessions, where there is no pod to delete, it is the whole cleanup. A foreground full-screen TUI (e.g. `vim`, `less`, `k9s`) may still orphan its shell, since it intercepts the control bytes used to ask the shell to exit.

### Configuration

```yaml
terminal:
  prefix: ctrl+a   # tmux-style prefix key (default)
  scrollback: 5000 # off-screen lines kept per terminal for scrolling (default)

exec:
  command: ["sh", "-c", "clear; (bash || ash || sh)"]  # shell for `s s` (default)
```

The captured-key set is just keymap bindings flagged `capture: true` (see [keymap.yaml](#keymapyaml)), so you can add your own. Under sustained I/O saturation aku drops keystrokes rather than blocking — a deliberate tradeoff that keeps the UI responsive when the remote shell can't keep up.

## Multi-cluster contexts

aku can drive several clusters at once. There is **no single "current cluster"** — every pane carries its own context and runs its own watches, and the **focused pane is the source of truth** (its context drives new-split defaults and the status bar). Different panes can be live on different clusters side by side.

### Making contexts available

By default aku uses the contexts in your active `$KUBECONFIG` / `~/.kube/config`. To pull in more, point `contexts.directories` at folders of kubeconfig files:

```yaml
contexts:
  directories:
    - ~/.kube/configs   # recursively scanned for kubeconfig files
    - ~/work/clusters   # every context found here becomes switchable
```

Directories are scanned recursively; files that aren't valid kubeconfigs (or contain no contexts) are skipped.

### Switching

| Keys | Action |
|------|--------|
| `gx` | Fuzzy **overlay picker** — moves every pane sharing the focused pane's context to the chosen cluster (a "context group" move) |
| `gX` | Opens the **contexts list** in the focused pane; `Enter` switches that pane |
| `oX` | Opens the contexts list in a **new split** — bring up another cluster side-by-side |

The contexts list is itself a resource view, with columns **NAME, CLUSTER, SERVER, STATUS, PANES**, where STATUS is `●` in use (green when connected, red when offline/degraded), `○` idle (no panes), or `◆` a pinned pseudo-context (the static `manifests` cluster — see [Manifest visualization](#manifest-visualization)). The `gx` overlay marks already-open contexts with `●` and their pane count, and highlights the focused pane's current context.

### Behavior

- All switches dial **asynchronously** off the UI loop, so an unreachable cluster never freezes aku — the pane shows an `⚠ offline` marker instead of hanging.
- Pane context labels appear only when more than one distinct context is open; when every pane shares one context, labels are hidden.
- On switch, a pane lands on the new context's default namespace (the kubeconfig `namespace:` field, else `default`). If the pane's current resource type doesn't exist on the new cluster, it shows an empty list and a brief status-bar note (this needs the cluster's API discovery to have populated first).

### Example: prod and staging side by side

1. `oX` opens the contexts list in a new split.
2. Move focus there (`shift+→`) and press `Enter` on `staging` — that pane is now live on staging while your original pane stays on prod.
3. From either pane, `gx` moves its whole context group to a different cluster.

## Manifest visualization

aku can read rendered Kubernetes manifests and present them as a **simulated, browsable cluster** — so you can inspect what a chart or overlay actually renders, interactively, before applying it, without a live cluster.

### Invoking

- **Pipe** a manifest stream — aku auto-detects it whenever stdin is not a TTY:

  ```bash
  helm template ./chart | aku
  kustomize build ./overlay | aku
  cat deploy.yaml | aku
  ```

- **`-f`** points at manifest files or directories (repeatable; directories are scanned for `*.yaml`/`*.yml`):

  ```bash
  aku -f ./manifests/
  aku -f deploy.yaml -f service.yaml
  ```

The manifests open in a pinned pseudo-context named **`manifests`** that coexists with your live clusters — `gx`/`gX`/`oX` all work, so you can bring a real cluster into a split alongside the simulated one. The initial pane opens at "All Namespaces" so a chart rendering into a non-`default` namespace isn't empty.

### What gets simulated

aku doesn't just list the YAML — it **fabricates the runtime that controllers would create**, so drill-down, health coloring, and resource lists all behave like a live cluster:

- **Deployment** → one ReplicaSet → N Pods (N = `spec.replicas`, default `1`; `0` ⇒ none)
- **StatefulSet** → ordinal Pods (`<sts>-0..N-1`)
- **DaemonSet** → one Pod
- **Job** → one Pod; **CronJob** → one Job → one Pod
- **Service** → an Endpoints object addressing the matching fabricated Pods

Fabricated objects carry **deterministic UIDs** (a stable hash of kind+namespace+name) so owner-ref drill-down (Deployment → ReplicaSet → Pods) resolves and runs are reproducible. Synthesized Pods are stamped **healthy/green** — `Running`, all containers ready, with a synthetic pod IP and start time; user-supplied objects that already carry a status keep it.

### Namespaces

Namespaced objects with no `metadata.namespace` default to `default` (or the value of `-n`/`--namespace`). aku then **fabricates a `Namespace` object** (phase `Active`) for every namespace referenced across the manifests, so the namespaces view and the namespace picker are populated even with no live API server.

### Provenance

When a manifest carries a `# Source: <path>` comment (as Helm emits), aku captures it into the `aku.dev/manifest-source` annotation on the object. Fabricated objects are annotated `aku.dev/manifest-source: synthesized`. The annotation shows up in the **YAML view** (`y`) for every kind — there is no separate column or describe line. It's a read-only preview, so the annotation simply appearing inline in the YAML is harmless.

### Blocked operations

The `manifests` context has no real API server, so anything that needs one is blocked and emits a toast reading `<op>: not available in manifest mode` — covering edit, scale, set image, rollout restart, delete, exec, debug, logs, and port-forward. (Helm operations toast `helm: no client` instead.) Browsing, drill-down, YAML, and describe all work.

### Reload

`Ctrl+r` re-reads `-f` files and rebuilds the simulated cluster in place, so editing a chart and re-running `helm template` into the same files is reflected on reload. A **piped stdin source can't be re-read** — `Ctrl+r` is a no-op there and emits a toast saying so.

### Limitations

- There are no real Pods, so logs, exec, and debug have nothing to attach to (they toast).
- Rendered manifests have **no real status** — synthesized health is fabricated, not observed; user objects that ship without a status are shown as-is.
- Only `Endpoints` is synthesized for Services; the `EndpointSlice` view is not populated.
- Unknown/CRD kinds get a **best-effort plural** for the resource name plus a warning (there's no live discovery to ask); they browse and show YAML, but nothing is synthesized for them.

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
| `Alt+z` | Toggle fullscreen zoom of the focused pane (captured: works over a live terminal too) |
| `Ctrl+r` | Reload all |
| `Ctrl+x` | Clear notifications (dismiss live toasts; history kept) |
| `/` | Search (regex) |
| `\|` / `Ctrl+/` | Filter (regex) |

### Copy & Open

The `c` chord copies or opens whatever the focused pane is showing. Copies attempt the native OS clipboard (best-effort) and always emit an OSC52 escape sequence, so `cc`/`cy` reach your *local* clipboard even over SSH (on an OSC52-capable terminal). Each action shows a confirmation toast.

| Keys | Action |
|------|--------|
| `c c` | Copy current — the selected resource name(s) when the list is focused (multiple marked rows are newline-joined), the buffer text in a YAML/describe panel, or the buffered log lines in a logs panel |
| `c y` | Copy YAML — the selected resource(s)' live YAML (multiple marked rows joined by `---`), regardless of which pane is focused |
| `c o` | Open in the editor (`$KUBE_EDITOR`, then `$EDITOR`, then `vi`) — resource list: the selected resource(s)' YAML via a temp file (multiple marked rows joined by `---`, like `c y`); YAML/describe panel: the buffer; logs: saves to the same destination as `Ctrl+s`, opens it, and copies the saved path to the clipboard |

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
| `g + p/d/s/v/c/n/m` | Go to pods/deployments/secrets/services/configmaps/namespaces/aku-messages |
| `o + p/d/s/v/c` | Open split: pods/deployments/secrets/services/configmaps |
| `gX` / `oX` | Open the contexts list (`g` in current pane, `o` in a new split) |
| `S + n/N/a/s` | Sort by name/namespace/age/status |

### Detail Panel

| Key | Action |
|-----|--------|
| `Tab` | Back to list |
| `w` | Toggle word wrap |
| `x` | Resolve env variables (for pods also reveals volume-mounted Secrets/ConfigMaps, projected sources, and imagePullSecrets — docker-config: registry+username only, never the password/token; non-docker-config pull secrets fall back to listing data keys with no values) |
| `r` | Refresh |
| `y` | Return to manifest YAML (resets `Values (user)`/`Values (all)` variant on helm releases) |

### Logs View

| Key | Action |
|-----|--------|
| `a` | Toggle autoscroll |
| `s` | Toggle syntax highlighting |
| `C` | Select container |
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
| `x` | Pods, Secrets, Containers, Deployments, StatefulSets, DaemonSets | Resolve/show env variables (list and detail); for pods also reveals volume-mounted Secrets/ConfigMaps, projected sources, and imagePullSecrets (docker-config: registry+username only, never the password/token; non-docker-config pull secrets fall back to listing data keys with no values) |
| `f` | Pods, Containers | Port forward |
| `ss` | Pods, Containers | Exec (live terminal pane) |
| `sd` | Pods, Containers, Nodes | Debug container (live terminal pane) |
| `sp` | Pods, Containers, Nodes | Privileged debug (live terminal pane) |

#### Set Image (`i`)

The Set Image overlay shows one row per container (init containers included). Each row has an image text input plus a pull-policy cycle that toggles between `Always`, `IfNotPresent`, `Never`, and `(default)`.

- `Tab`/`Down` step, per container, through the image input then its pull-policy cycle, and finally the `Yes`/`No` buttons, wrapping back to the first container; `Shift+Tab`/`Up` walk the same order in reverse.
- When a pull-policy cycle is focused, `Left` selects the previous value and `Right` or `Space` the next.
- Image and policy are diffed independently, so changing only one leaves the other untouched.

Reverting a previously-set policy back to `(default)` is intentionally a no-op: Kubernetes cannot reliably clear `imagePullPolicy` through a merge patch, so the revert is suppressed (any image change on the same row still applies).

### Terminal Pane

A focused live terminal forwards most keystrokes to the shell; use the prefix (default `Ctrl+a`) to navigate. The captured keys `Alt+z` and `Shift+arrows` are handled by aku even over a live terminal.

| Key | Action |
|-----|--------|
| `Alt+z` | Toggle fullscreen zoom (captured; top bar shows an `alt+z: exit zoom` hint) |
| `Shift+←/→/↑/↓` | Move focus between panes (captured; zoom follows the focused split) |
| `<prefix>` then `h/j/k/l` or arrows | Move focus |
| `<prefix>` then `x` / `q` / `Ctrl+w` | Close pane |
| `Ctrl+w` | Close pane (exited pane only; on a live pane a bare `Ctrl+w` is sent to the shell — use `<prefix>` then `Ctrl+w` to close) |
| `<prefix> <prefix>` | Send a literal prefix byte to the shell |
| `<prefix>` then `PgUp` / `PgDn` | Scroll scrollback (an exited pane also accepts bare `PgUp` / `PgDn`) |

The prefix key is configurable via `terminal.prefix` in `config.yaml`. The captured keys (`alt+z`, `shift+arrows`) can be customized via the `capture:` field in `keymap.yaml`.

## Name

The name is a reference to [Aku](https://en.wikipedia.org/wiki/Aku_(Samurai_Jack)) from Samurai Jack — and also stands for **A**nother **K**8s **U**I.

## License

MIT
