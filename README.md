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
- Helm release management with values editing, rollback, and chart switching
- Drill-down navigation between related resources (deployment → replicaset → pods → containers)

**Views**
- YAML view with syntax highlighting (managedFields stripped)
- Describe view with events and environment variable resolution
- Live log streaming with time range presets, container selection, and autoscroll
- Split panes with independent namespace, filter, and cursor per pane
- Zoom to full-screen any split or detail panel

**Operations**
- Edit resources in your `$EDITOR` with automatic retry on validation errors
- Exec into containers
- Ephemeral debug containers (pods and nodes, with optional privileged mode)
- Port forwarding with live status tracking
- Update container images across workloads
- Rollout restart for deployments and pods
- Multi-select resources for bulk delete
- Helm values editing, rollback to any revision, and chart reference updates

**Navigation**
- Vim-style keybindings with multi-key sequences (`gg`, `gp`, `gd`, etc.)
- Fuzzy resource picker (`:`) and namespace picker (`Ctrl+n`)
- Regex search (`/`) and filter (`Ctrl+/`) in both list and detail views
- Column sorting by name, namespace, age, status, or kind
- Fully customizable keybindings via YAML

## Installation

### From source

```bash
go install github.com/aohoyd/aku@latest
```

### Build locally

```bash
git clone https://github.com/aohoyd/aku.git
cd aku
go build -o aku .
```

## Usage

```bash
aku
```

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
```

### keymap.yaml

```yaml
bindings:
  # Add a custom binding
  - key: "ctrl+l"
    help: "logs"
    command: "view-logs-focused"
    scope: "resource-list"
    for: ["pods"]
    visible: true

  # Multi-key sequence
  - key: "g"
    scope: "resource-list"
    keys:
      - key: "i"
        help: "ingresses"
        command: "goto-ingresses"
```

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

# Log line highlighting rules
highlights:
  - pattern: "ERROR|FATAL"
    fg: "#EF4444"
    bold: true
  - pattern: "WARN"
    fg: "#F59E0B"
```

## Key Bindings

### Global

| Key | Action |
|-----|--------|
| `q` | Quit / close overlay |
| `Ctrl+n` | Namespace picker |
| `:` | Resource picker |
| `?` | Help overlay |
| `y` | YAML view |
| `d` | Describe view |
| `e` | Edit resource |
| `Z` | Toggle zoom |
| `Ctrl+r` | Reload all |

### Resource List

| Key | Action |
|-----|--------|
| `j/k` | Cursor up/down |
| `gg` / `G` | Top / bottom |
| `Enter` | Drill down / open detail |
| `Tab` / `Shift+Tab` | Next / prev split pane |
| `/` | Search (regex) |
| `Ctrl+/` | Filter (regex) |
| `Space` | Toggle select |
| `Ctrl+a` | Select all |
| `Ctrl+d` | Delete selected |
| `g + p/d/s/v/c/n` | Go to pods/deployments/secrets/services/configmaps/namespaces |
| `h + p/d/s/v/c` | Open split: pods/deployments/secrets/services/configmaps |
| `S + n/N/a/s` | Sort by name/namespace/age/status |

### Detail Panel

| Key | Action |
|-----|--------|
| `h` / `Left` | Back to list |
| `w` | Toggle word wrap |
| `H/L` | Scroll left/right |
| `x` | Resolve env variables |
| `r` | Refresh |

### Resource-Specific

| Key | Resource | Action |
|-----|----------|--------|
| `l` | Pods, Containers | View logs |
| `a` | Pods, Containers | Toggle autoscroll (logs) |
| `c` | Pods, Containers | Select container (logs) |
| `t` | Pods, Containers | Time range (logs) |
| `i` | Pods, Deployments, StatefulSets, DaemonSets | Set image |
| `R` | Deployments, Pods | Rollout restart |
| `R` | Helm releases | Rollback |
| `C` | Helm releases | Set chart |
| `pf` | Pods, Containers | Port forward |
| `ss` | Pods, Containers | Exec |
| `sd` | Pods, Containers, Nodes | Debug container |
| `sp` | Pods, Containers, Nodes | Privileged debug |

## Name

The name is a reference to [Aku](https://en.wikipedia.org/wiki/Aku_(Samurai_Jack)) from Samurai Jack — and also stands for **A**nother **K**8s **U**I.

## License

MIT
