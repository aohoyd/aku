package highlight

import (
	"encoding/json"
	"strings"

	"github.com/valyala/fastjson"
)

// JSONHighlighter colorizes JSON fragments found within a line.
// It runs in "raw" mode (first in the pipeline) and colorizes only structural
// tokens: braces, brackets, colons, commas, and key names. Values (strings,
// numbers, booleans, null) are left uncolored so downstream highlighters can
// handle them.
type JSONHighlighter struct {
	keyPainter    Painter
	markerPainter Painter
	parser        fastjson.Parser
}

// NewJSONHighlighter creates a JSONHighlighter with the given painters.
// keyPainter colors key name text; markerPainter colors structural tokens
// ({}[],:) and the quote characters around keys.
func NewJSONHighlighter(key, marker Painter) *JSONHighlighter {
	return &JSONHighlighter{
		keyPainter:    key,
		markerPainter: marker,
	}
}

// Highlight scans the line for JSON fragments and colorizes them.
// Non-JSON segments are returned as plain text. If no JSON is found,
// the original string (same pointer) is returned.
func (h *JSONHighlighter) Highlight(line string) string {
	// Early-exit guard: if no braces or brackets, skip.
	if strings.IndexByte(line, '{') == -1 && strings.IndexByte(line, '[') == -1 {
		return line
	}

	fragments := h.findJSONFragments(line)
	if len(fragments) == 0 {
		return line
	}

	var sb strings.Builder
	sb.Grow(len(line) + 64)
	prev := 0
	for _, frag := range fragments {
		if prev < frag.start {
			sb.WriteString(line[prev:frag.start])
		}
		h.colorizeJSON(&sb, line[frag.start:frag.end])
		prev = frag.end
	}
	if prev < len(line) {
		sb.WriteString(line[prev:])
	}
	return sb.String()
}

// jsonFragment records the byte offsets of a valid JSON fragment within a line.
type jsonFragment struct {
	start, end int
}

// findJSONFragments scans a line for valid JSON objects ({...}) and arrays ([...]).
// It tracks nesting depth and handles quoted strings (including escaped quotes).
// Only candidates that pass fastjson parsing are returned.
func (h *JSONHighlighter) findJSONFragments(line string) []jsonFragment {
	var fragments []jsonFragment
	i := 0
	for i < len(line) {
		ch := line[i]
		if ch != '{' && ch != '[' {
			i++
			continue
		}
		// Found a potential JSON start
		open := ch
		var close byte
		if open == '{' {
			close = '}'
		} else {
			close = ']'
		}
		depth := 1
		j := i + 1
		for j < len(line) && depth > 0 {
			c := line[j]
			if c == '"' {
				// Skip quoted string
				j++
				for j < len(line) {
					if line[j] == '\\' {
						j += 2 // skip escaped character
						continue
					}
					if line[j] == '"' {
						j++
						break
					}
					j++
				}
				continue
			}
			if c == open || (open == '{' && c == '[') || (open == '[' && c == '{') {
				depth++
			} else if c == close || (open == '{' && c == ']') || (open == '[' && c == '}') {
				depth--
			}
			j++
		}
		if depth == 0 {
			candidate := line[i:j]
			if _, err := h.parser.Parse(candidate); err == nil {
				fragments = append(fragments, jsonFragment{start: i, end: j})
				i = j
				continue
			}
		}
		i++
	}
	return fragments
}

// colorizeJSON parses a raw JSON string and writes colorized output to sb.
func (h *JSONHighlighter) colorizeJSON(sb *strings.Builder, raw string) {
	v, err := h.parser.Parse(raw)
	if err != nil {
		sb.WriteString(raw)
		return
	}
	h.writeJSONValue(sb, v)
}

// writeJSONValue recursively writes a fastjson value to the builder with
// appropriate coloring. Keys and structural tokens are colored; values are plain.
func (h *JSONHighlighter) writeJSONValue(sb *strings.Builder, v *fastjson.Value) {
	switch v.Type() {
	case fastjson.TypeObject:
		h.markerPainter.WriteTo(sb, "{")
		sb.WriteString(" ")
		obj, _ := v.Object()
		i := 0
		obj.Visit(func(key []byte, val *fastjson.Value) {
			if i > 0 {
				h.markerPainter.WriteTo(sb, ",")
				sb.WriteString(" ")
			}
			// Quote characters around keys use markerPainter, key text uses keyPainter
			h.markerPainter.WriteTo(sb, `"`)
			h.keyPainter.WriteTo(sb, string(key))
			h.markerPainter.WriteTo(sb, `"`)
			h.markerPainter.WriteTo(sb, ":")
			sb.WriteString(" ")
			h.writeJSONValue(sb, val)
			i++
		})
		sb.WriteString(" ")
		h.markerPainter.WriteTo(sb, "}")

	case fastjson.TypeArray:
		h.markerPainter.WriteTo(sb, "[")
		arr, _ := v.Array()
		for i, elem := range arr {
			if i > 0 {
				h.markerPainter.WriteTo(sb, ",")
				sb.WriteString(" ")
			}
			h.writeJSONValue(sb, elem)
		}
		h.markerPainter.WriteTo(sb, "]")

	case fastjson.TypeString:
		s := v.GetStringBytes()
		sb.WriteString(quoteJSONString(string(s)))

	case fastjson.TypeNumber:
		sb.Write(v.MarshalTo(nil))

	case fastjson.TypeTrue:
		sb.WriteString("true")

	case fastjson.TypeFalse:
		sb.WriteString("false")

	case fastjson.TypeNull:
		sb.WriteString("null")
	}
}

// quoteJSONString wraps s in double quotes with JSON escaping.
func quoteJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
