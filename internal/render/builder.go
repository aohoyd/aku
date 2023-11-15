package render

import (
	"fmt"
	"slices"
	"strings"
)

// elemKind distinguishes the type of element stored in a Builder.
type elemKind int

const (
	elemSection elemKind = iota
	elemKV
	elemRaw
)

// Element represents a single renderable item in the describe output.
type Element struct {
	kind      elemKind
	level     int
	key       string
	values    []string
	vkind     ValueKind
	unaligned bool
	padWidth  int
}

// KVOption is a functional option for KV methods on Builder.
type KVOption func(*Element)

// WithKind sets the ValueKind for styling the value portion of a KV element.
func WithKind(k ValueKind) KVOption {
	return func(e *Element) {
		e.vkind = k
	}
}

// Unaligned marks the KV element as excluded from per-section alignment computation.
// The key and value are rendered with a single space separator instead.
func Unaligned() KVOption {
	return func(e *Element) {
		e.unaligned = true
	}
}

// Builder accumulates typed elements and renders them with dynamic per-section alignment.
type Builder struct {
	elems []Element
}

// NewBuilder creates a new empty Builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// Section appends a section header element. It renders as "indent + name + :\n".
func (b *Builder) Section(level int, name string) *Builder {
	b.elems = append(b.elems, Element{
		kind:  elemSection,
		level: level,
		key:   name,
	})
	return b
}

// KV appends a key-value element with optional KVOption modifiers.
func (b *Builder) KV(level int, key, value string, opts ...KVOption) *Builder {
	e := Element{
		kind:   elemKV,
		level:  level,
		key:    key,
		values: []string{value},
		vkind:  ValueDefault,
	}
	for _, opt := range opts {
		opt(&e)
	}
	b.elems = append(b.elems, e)
	return b
}

// KVStyled appends a key-value element with a specific ValueKind for styling.
func (b *Builder) KVStyled(level int, kind ValueKind, key, value string) *Builder {
	b.elems = append(b.elems, Element{
		kind:   elemKV,
		level:  level,
		key:    key,
		values: []string{value},
		vkind:  kind,
	})
	return b
}

// KVMulti appends a key with a sorted map of key=value pairs. The first value appears
// on the same line as the key; subsequent values appear on continuation lines aligned
// to the same column. A nil or empty map renders as "<none>".
func (b *Builder) KVMulti(level int, key string, m map[string]string, opts ...KVOption) *Builder {
	if len(m) == 0 {
		return b.KV(level, key, "<none>")
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	vals := make([]string, len(keys))
	for i, k := range keys {
		if v := m[k]; v != "" {
			k += "=" + v
		}
		vals[i] = k
	}
	e := Element{
		kind:   elemKV,
		level:  level,
		key:    key,
		values: vals,
		vkind:  ValueDefault,
	}
	for _, opt := range opts {
		opt(&e)
	}
	b.elems = append(b.elems, e)
	return b
}

// RawLine appends an unstyled line of text at the given indentation level.
func (b *Builder) RawLine(level int, text string) *Builder {
	b.elems = append(b.elems, Element{
		kind:  elemRaw,
		level: level,
		key:   text,
	})
	return b
}

// AppendAt appends existing builder's elements at the given indentation level.
func (b *Builder) AppendAt(level int, i *Builder) *Builder {
	ne := slices.Clone(i.elems)
	for i := range ne {
		ne[i].level += level
	}
	b.elems = append(b.elems, ne...)
	return b
}

// Build performs a two-pass render and returns a Content holding (raw, display) output.
// raw contains no ANSI escape codes; display contains styled output.
// The invariant ansi.Strip(display) == raw always holds.
func (b *Builder) Build() Content {
	if len(b.elems) == 0 {
		return Content{}
	}

	computeAlignmentWidths(b.elems)

	var raw, color strings.Builder
	for _, e := range b.elems {
		renderElement(&raw, &color, e)
	}

	return Content{Raw: raw.String(), Display: color.String()}
}

// computeAlignmentWidths calculates the pad width for every element.
// Elements are grouped into blocks separated by section headers.
// Within each block, the pad width is derived from the widest non-unaligned KV.
func computeAlignmentWidths(elems []Element) {
	// Find block boundaries: a new block starts at the beginning and at each section element.
	blockStart := 0
	for i := 0; i <= len(elems); i++ {
		if i == len(elems) || (i > blockStart && elems[i].kind == elemSection) {
			// Process block [blockStart, i)
			setBlockWidths(elems, blockStart, i)
			blockStart = i
		}
	}
}

// setBlockWidths computes and assigns pad widths for elements in [start, end).
func setBlockWidths(elems []Element, start, end int) {
	maxVis := 0
	for i := start; i < end; i++ {
		e := elems[i]
		if e.kind == elemKV && !e.unaligned {
			vis := e.level*2 + len(e.key)
			if vis > maxVis {
				maxVis = vis
			}
		}
	}

	padWidth := maxVis + 3 // at least 2 spaces and `:` after the widest key

	for i := start; i < end; i++ {
		elems[i].padWidth = padWidth
	}
}

// renderElement writes a single element to both raw and color builders.
func renderElement(raw, color *strings.Builder, e Element) {
	indent := ""
	if e.level > 0 {
		indent = strings.Repeat("  ", e.level)
	}

	switch e.kind {
	case elemSection:
		line := indent + e.key + ":\n"
		raw.WriteString(line)
		color.WriteString(indent)
		color.WriteString(writerKeyStyle.Render(e.key + ":"))
		color.WriteString("\n")

	case elemKV:
		if e.unaligned {
			renderUnalignedKV(raw, color, e, indent)
			return
		}
		renderAlignedKV(raw, color, e, indent)

	case elemRaw:
		line := indent + e.key + "\n"
		raw.WriteString(line)
		color.WriteString(line)
	}
}

// renderUnalignedKV renders a KV that opted out of alignment: "indent + key + space + value".
func renderUnalignedKV(raw, color *strings.Builder, e Element, indent string) {
	value := ""
	if len(e.values) > 0 {
		value = e.values[0]
	}

	rawLine := indent + e.key + " " + value + "\n"
	raw.WriteString(rawLine)

	color.WriteString(indent)
	color.WriteString(writerKeyStyle.Render(e.key))
	color.WriteString(" ")
	if value != "" {
		valStyle := writerValueStyleForKind(e.vkind)
		color.WriteString(valStyle.Render(value))
	}
	color.WriteString("\n")
}

// renderAlignedKV renders a KV with padding for alignment within its block.
// Non-empty keys automatically get a colon appended before padding.
func renderAlignedKV(raw, color *strings.Builder, e Element, indent string) {
	// Auto-append colon for non-empty keys.
	displayKey := e.key
	if displayKey != "" {
		displayKey += ":"
	}

	// localPad is the pad width relative to this element's indentation.
	localPad := max(e.padWidth-e.level*2, len(displayKey)+1)

	paddedKey := fmt.Sprintf("%-*s", localPad, displayKey)

	// First line: indent + paddedKey + values[0]
	firstVal := ""
	if len(e.values) > 0 {
		firstVal = e.values[0]
	}

	raw.WriteString(indent)
	raw.WriteString(paddedKey)
	raw.WriteString(firstVal)
	raw.WriteString("\n")

	color.WriteString(indent)
	color.WriteString(writerKeyStyle.Render(paddedKey))
	if firstVal != "" {
		valStyle := writerValueStyleForKind(e.vkind)
		color.WriteString(valStyle.Render(firstVal))
	}
	color.WriteString("\n")

	// Continuation lines for multi-value KVs.
	if len(e.values) > 1 {
		contPad := strings.Repeat(" ", e.padWidth)
		for _, v := range e.values[1:] {
			raw.WriteString(contPad)
			raw.WriteString(v)
			raw.WriteString("\n")

			color.WriteString(contPad)
			valStyle := writerValueStyleForKind(e.vkind)
			color.WriteString(valStyle.Render(v))
			color.WriteString("\n")
		}
	}
}
