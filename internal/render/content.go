package render

// Content holds the raw (plain-text) and display (ANSI-styled) representations
// of rendered content. The invariant ansi.Strip(c.Display) == c.Raw always holds.
type Content struct {
	Raw     string
	Display string
}

// Append concatenates other to c and returns a new Content.
// If either operand is empty, the other is returned unchanged.
// No separator is inserted because Builder.Build output always ends with '\n'.
func (c Content) Append(other Content) Content {
	if other.Raw == "" {
		return c
	}
	if c.Raw == "" {
		return other
	}
	return Content{
		Raw:     c.Raw + other.Raw,
		Display: c.Display + other.Display,
	}
}
