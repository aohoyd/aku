package highlight

// Pipeline applies a sequence of Highlighters to a line.
// Each step is either "guarded" (applied only to unhighlighted regions via
// ApplyToUnhighlighted) or "raw" (applied to the entire line including ANSI
// codes from previous steps).
type Pipeline struct {
	steps []step
}

type step struct {
	highlighter Highlighter
	raw         bool // true = apply to entire line; false = skip highlighted regions
}

// Highlight applies all highlighters in order. Returns the original string
// (same pointer) if no step modified it.
func (p *Pipeline) Highlight(line string) string {
	if p == nil {
		return line
	}
	result := line
	for _, s := range p.steps {
		if s.raw {
			result = s.highlighter.Highlight(result)
		} else {
			result = ApplyToUnhighlighted(result, s.highlighter)
		}
	}
	return result
}

// PipelineBuilder constructs a Pipeline step by step.
type PipelineBuilder struct {
	steps []step
}

// NewPipelineBuilder returns a new empty PipelineBuilder.
func NewPipelineBuilder() *PipelineBuilder {
	return &PipelineBuilder{}
}

// Add appends a guarded step — the highlighter is applied only to text
// not already wrapped in ANSI escape sequences.
func (b *PipelineBuilder) Add(h Highlighter) *PipelineBuilder {
	b.steps = append(b.steps, step{highlighter: h, raw: false})
	return b
}

// AddRaw appends a raw step — the highlighter receives the full line
// including any ANSI codes from previous steps. Use this for highlighters
// that need to see structure (e.g., JSON, quotes).
func (b *PipelineBuilder) AddRaw(h Highlighter) *PipelineBuilder {
	b.steps = append(b.steps, step{highlighter: h, raw: true})
	return b
}

// Build returns the constructed Pipeline.
func (b *PipelineBuilder) Build() *Pipeline {
	steps := make([]step, len(b.steps))
	copy(steps, b.steps)
	return &Pipeline{steps: steps}
}
