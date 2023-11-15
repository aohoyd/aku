package render

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"
)

// YAML renders a map[string]any as syntax-colored YAML.
// It returns a Content holding the raw (plain) and display (ANSI-colored) text.
func YAML(m map[string]any) (Content, error) {
	if len(m) == 0 {
		return Content{}, nil
	}
	var rawBuf, colorBuf strings.Builder
	renderMap(&rawBuf, &colorBuf, m, 0)
	return Content{Raw: rawBuf.String(), Display: colorBuf.String()}, nil
}

func renderMap(raw, color *strings.Builder, m map[string]any, indent int) {
	keys := sortedKeysAtDepth(m, indent)
	prefix := strings.Repeat(" ", indent)
	for _, k := range keys {
		v := m[k]
		keyStr := k + ":"
		switch child := v.(type) {
		case map[string]any:
			writeKey(raw, color, prefix, keyStr)
			raw.WriteByte('\n')
			color.WriteByte('\n')
			renderMap(raw, color, child, indent+2)
		case []any:
			writeKey(raw, color, prefix, keyStr)
			raw.WriteByte('\n')
			color.WriteByte('\n')
			renderSlice(raw, color, child, indent+2)
		default:
			writeKeyValue(raw, color, prefix, keyStr, v)
		}
	}
}

func renderSlice(raw, color *strings.Builder, s []any, indent int) {
	prefix := strings.Repeat(" ", indent)
	for _, item := range s {
		switch child := item.(type) {
		case map[string]any:
			renderSeqMap(raw, color, child, indent)
		case []any:
			writeMarker(raw, color, prefix)
			raw.WriteByte('\n')
			color.WriteByte('\n')
			renderSlice(raw, color, child, indent+2)
		default:
			writeMarkerValue(raw, color, prefix, item)
		}
	}
}

func renderSeqMap(raw, color *strings.Builder, m map[string]any, indent int) {
	prefix := strings.Repeat(" ", indent)
	keys := sortedKeys(m)
	for i, k := range keys {
		v := m[k]
		keyStr := k + ":"
		if i == 0 {
			writeMarkerKey(raw, color, prefix, keyStr)
			switch child := v.(type) {
			case map[string]any:
				raw.WriteByte('\n')
				color.WriteByte('\n')
				renderMap(raw, color, child, indent+4)
			case []any:
				raw.WriteByte('\n')
				color.WriteByte('\n')
				renderSlice(raw, color, child, indent+4)
			default:
				raw.WriteByte(' ')
				color.WriteByte(' ')
				writeScalar(raw, color, v)
				raw.WriteByte('\n')
				color.WriteByte('\n')
			}
		} else {
			linePrefix := prefix + "  "
			switch child := v.(type) {
			case map[string]any:
				writeKey(raw, color, linePrefix, keyStr)
				raw.WriteByte('\n')
				color.WriteByte('\n')
				renderMap(raw, color, child, indent+4)
			case []any:
				writeKey(raw, color, linePrefix, keyStr)
				raw.WriteByte('\n')
				color.WriteByte('\n')
				renderSlice(raw, color, child, indent+4)
			default:
				writeKeyValue(raw, color, linePrefix, keyStr, v)
			}
		}
	}
}

func writeKey(raw, color *strings.Builder, prefix, keyStr string) {
	raw.WriteString(prefix)
	color.WriteString(prefix)
	raw.WriteString(keyStr)
	color.WriteString(YAMLKeyStyle.Render(keyStr))
}

func writeKeyValue(raw, color *strings.Builder, prefix, keyStr string, v any) {
	raw.WriteString(prefix)
	color.WriteString(prefix)
	raw.WriteString(keyStr)
	color.WriteString(YAMLKeyStyle.Render(keyStr))
	raw.WriteByte(' ')
	color.WriteByte(' ')
	writeScalar(raw, color, v)
	raw.WriteByte('\n')
	color.WriteByte('\n')
}

func writeMarkerKey(raw, color *strings.Builder, prefix, keyStr string) {
	raw.WriteString(prefix)
	color.WriteString(prefix)
	raw.WriteString("- ")
	color.WriteString(YAMLMarkerStyle.Render("- "))
	raw.WriteString(keyStr)
	color.WriteString(YAMLKeyStyle.Render(keyStr))
}

func writeMarker(raw, color *strings.Builder, prefix string) {
	raw.WriteString(prefix)
	color.WriteString(prefix)
	raw.WriteString("- ")
	color.WriteString(YAMLMarkerStyle.Render("- "))
}

func writeMarkerValue(raw, color *strings.Builder, prefix string, v any) {
	writeMarker(raw, color, prefix)
	writeScalar(raw, color, v)
	raw.WriteByte('\n')
	color.WriteByte('\n')
}

func writeScalar(raw, color *strings.Builder, v any) {
	switch typed := v.(type) {
	case nil:
		raw.WriteString("null")
		color.WriteString(YAMLNullStyle.Render("null"))
	case bool:
		s := strconv.FormatBool(typed)
		raw.WriteString(s)
		color.WriteString(YAMLBoolStyle.Render(s))
	case int64:
		s := strconv.FormatInt(typed, 10)
		raw.WriteString(s)
		color.WriteString(YAMLNumberStyle.Render(s))
	case float64:
		s := formatFloat(typed)
		raw.WriteString(s)
		color.WriteString(YAMLNumberStyle.Render(s))
	case string:
		s := quoteIfNeeded(typed)
		raw.WriteString(s)
		color.WriteString(YAMLStringStyle.Render(s))
	default:
		s := fmt.Sprintf("%v", typed)
		raw.WriteString(s)
		color.WriteString(s)
	}
}

func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func quoteIfNeeded(s string) string {
	if s == "" || s == "true" || s == "false" || s == "null" ||
		s == "True" || s == "False" || s == "~" || s == "yes" || s == "no" ||
		strings.ContainsAny(s, ":#{}[]&*?|>!'\"\n") ||
		strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") ||
		strings.HasPrefix(s, "-") || strings.HasPrefix(s, "@") {
		data, err := sigsyaml.Marshal(s)
		if err != nil {
			return fmt.Sprintf("%q", s)
		}
		return strings.TrimRight(string(data), "\n")
	}
	return s
}

var topLevelOrder = map[string]int{
	"apiVersion": 0,
	"kind":       1,
	"metadata":   2,
	"spec":       3,
	"status":     4,
}

func sortedKeysAtDepth(m map[string]any, indent int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	if indent == 0 {
		slices.SortFunc(keys, func(a, b string) int {
			oa, oka := topLevelOrder[a]
			ob, okb := topLevelOrder[b]
			if oka && okb {
				return cmp.Compare(oa, ob)
			}
			if oka {
				return -1
			}
			if okb {
				return 1
			}
			return cmp.Compare(a, b)
		})
	} else {
		slices.Sort(keys)
	}
	return keys
}

func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}
