package render

// MapOffsets translates byte-offset pairs from raw (plain-text) space
// to display (ANSI-colored) space.
func MapOffsets(rawText, displayText string, rawOffsets [][]int) [][]int {
	if len(rawOffsets) == 0 || rawText == displayText {
		return rawOffsets
	}
	return mapOffsetsToDisplay(rawText, displayText, rawOffsets)
}

// mapOffsetsToDisplay translates byte offsets from raw-space to display-space
// by walking both strings in lockstep and skipping ANSI escape sequences in display.
func mapOffsetsToDisplay(raw, display string, rawOffsets [][]int) [][]int {
	mapping := make([]int, len(raw)+1)
	ri, di := 0, 0
	for ri < len(raw) && di < len(display) {
		for di < len(display) && display[di] == '\x1b' {
			di++
			for di < len(display) && !isEscTerminator(display[di]) {
				di++
			}
			if di < len(display) {
				di++
			}
		}
		mapping[ri] = di
		ri++
		di++
	}
	mapping[len(raw)] = len(display)

	out := make([][]int, 0, len(rawOffsets))
	for _, pair := range rawOffsets {
		if len(pair) < 2 || pair[0] > len(raw) || pair[1] > len(raw) {
			continue
		}
		out = append(out, []int{mapping[pair[0]], mapping[pair[1]]})
	}
	return out
}

// isEscTerminator reports whether b is the final byte of an ANSI escape sequence.
func isEscTerminator(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
