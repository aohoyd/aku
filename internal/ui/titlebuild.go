package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const maxPatternRunes = 20

// BuildPanelTitle assembles the complete rendered panel title from a base title
// string and optional filter/search patterns. filterPattern and searchPattern
// are raw pattern strings — empty means inactive. panelWidth is the total
// rendered width of the panel border including corners.
func BuildPanelTitle(base, filterPattern, searchPattern string, panelWidth int, inlineInput string) string {
	return BuildPanelTitleWithPrefix("", base, filterPattern, searchPattern, panelWidth, inlineInput)
}

// BuildPanelTitleWithPrefix assembles a panel title with a dimmed prefix
// (e.g. namespace) followed by a bright base title and optional patterns.
func BuildPanelTitleWithPrefix(prefix, base, filterPattern, searchPattern string, panelWidth int, inlineInput string) string {
	maxTitleCols := panelWidth - 3
	if maxTitleCols < 4 {
		return TitleStyle.Render(prefix + base)
	}

	var prefixRendered string
	if prefix != "" {
		prefixRendered = TitleIndicatorStyle.Render(prefix)
	}
	baseRendered := TitleStyle.Render(base)
	totalCols := lipgloss.Width(prefixRendered) + lipgloss.Width(baseRendered)

	var suffix string
	if inlineInput != "" {
		suffix = inlineInput
	} else {
		suffix = buildPatternSuffix(filterPattern, searchPattern)
	}
	if suffix == "" {
		return prefixRendered + baseRendered
	}

	remaining := maxTitleCols - totalCols
	if remaining < 3 {
		return prefixRendered + baseRendered
	}

	if ansi.StringWidth(suffix) > remaining {
		suffix = ansi.Truncate(suffix, remaining-1, "…")
	}

	if inlineInput != "" {
		return prefixRendered + baseRendered + suffix
	}
	return prefixRendered + baseRendered + TitleIndicatorStyle.Render(suffix)
}

// buildPatternSuffix constructs the raw annotation string.
// Returns "" when both patterns are inactive.
func buildPatternSuffix(filterPattern, searchPattern string) string {
	if filterPattern == "" && searchPattern == "" {
		return ""
	}
	var sb strings.Builder
	if filterPattern != "" {
		fmt.Fprintf(&sb, "|%s|", truncatePattern(filterPattern, maxPatternRunes))
	}
	if searchPattern != "" {
		if filterPattern != "" {
			sb.WriteString(" ")
		}
		fmt.Fprintf(&sb, "/%s/", truncatePattern(searchPattern, maxPatternRunes))
	}
	sb.WriteString(" ")
	return sb.String()
}

// truncatePattern caps a pattern at maxRunes, appending "…" when truncated.
func truncatePattern(pattern string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(pattern)
	if len(runes) <= maxRunes {
		return pattern
	}
	return string(runes[:maxRunes-1]) + "…"
}
