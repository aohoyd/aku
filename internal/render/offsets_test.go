package render

import (
	"reflect"
	"testing"
)

func TestMapOffsets(t *testing.T) {
	tests := []struct {
		name        string
		rawText     string
		displayText string
		rawOffsets  [][]int
		want        [][]int
	}{
		{
			name:        "identity mapping no ANSI",
			rawText:     "hello world",
			displayText: "hello world",
			rawOffsets:  [][]int{{0, 5}, {6, 11}},
			want:        [][]int{{0, 5}, {6, 11}},
		},
		{
			name:        "simple color code at start",
			rawText:     "hello",
			displayText: "\x1b[31mhello\x1b[0m",
			rawOffsets:  [][]int{{0, 5}},
			want:        [][]int{{5, 14}},
		},
		{
			name:        "color code wrapping partial text",
			rawText:     "hello world",
			displayText: "\x1b[32mhello\x1b[0m world",
			rawOffsets:  [][]int{{0, 5}},
			want:        [][]int{{5, 14}},
		},
		{
			name:        "offset after color code",
			rawText:     "hello world",
			displayText: "\x1b[32mhello\x1b[0m world",
			rawOffsets:  [][]int{{6, 11}},
			want:        [][]int{{15, 20}},
		},
		{
			name:        "multiple color codes",
			rawText:     "ab",
			displayText: "\x1b[31ma\x1b[0m\x1b[32mb\x1b[0m",
			rawOffsets:  [][]int{{0, 1}, {1, 2}},
			want:        [][]int{{5, 15}, {15, 20}},
		},
		{
			name:        "nested escape sequences",
			rawText:     "text",
			displayText: "\x1b[1m\x1b[31mtext\x1b[0m\x1b[0m",
			rawOffsets:  [][]int{{0, 4}},
			want:        [][]int{{9, 21}},
		},
		{
			name:        "offset at start of string",
			rawText:     "abc",
			displayText: "abc",
			rawOffsets:  [][]int{{0, 1}},
			want:        [][]int{{0, 1}},
		},
		{
			name:        "offset at end of string",
			rawText:     "abc",
			displayText: "\x1b[31mabc\x1b[0m",
			rawOffsets:  [][]int{{2, 3}},
			want:        [][]int{{7, 12}},
		},
		{
			name:        "empty input strings",
			rawText:     "",
			displayText: "",
			rawOffsets:  [][]int{{0, 0}},
			want:        [][]int{{0, 0}},
		},
		{
			name:        "nil offsets",
			rawText:     "hello",
			displayText: "\x1b[31mhello\x1b[0m",
			rawOffsets:  nil,
			want:        nil,
		},
		{
			name:        "empty offsets slice",
			rawText:     "hello",
			displayText: "\x1b[31mhello\x1b[0m",
			rawOffsets:  [][]int{},
			want:        [][]int{},
		},
		{
			name:        "offset beyond string length skipped",
			rawText:     "abc",
			displayText: "\x1b[31mabc\x1b[0m",
			rawOffsets:  [][]int{{0, 10}},
			want:        [][]int{},
		},
		{
			name:        "pair with insufficient elements skipped",
			rawText:     "abc",
			displayText: "\x1b[31mabc\x1b[0m",
			rawOffsets:  [][]int{{0}},
			want:        [][]int{},
		},
		{
			name:        "mixed valid and invalid offsets",
			rawText:     "abc",
			displayText: "\x1b[31mabc\x1b[0m",
			rawOffsets:  [][]int{{0, 1}, {0, 100}, {1, 2}},
			want:        [][]int{{5, 6}, {6, 7}},
		},
		{
			name:        "offset spanning entire raw text",
			rawText:     "hello",
			displayText: "\x1b[31mhello\x1b[0m",
			rawOffsets:  [][]int{{0, 5}},
			want:        [][]int{{5, 14}},
		},
		{
			name:        "zero-length offset pair",
			rawText:     "hello",
			displayText: "\x1b[31mhello\x1b[0m",
			rawOffsets:  [][]int{{2, 2}},
			want:        [][]int{{7, 7}},
		},
		{
			name:        "SGR with semicolons",
			rawText:     "bold",
			displayText: "\x1b[1;31mbold\x1b[0m",
			rawOffsets:  [][]int{{0, 4}},
			want:        [][]int{{7, 15}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapOffsets(tt.rawText, tt.displayText, tt.rawOffsets)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapOffsets() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsEscTerminator(t *testing.T) {
	tests := []struct {
		b    byte
		want bool
	}{
		{'m', true},
		{'A', true},
		{'Z', true},
		{'a', true},
		{'z', true},
		{'H', true},
		{'0', false},
		{';', false},
		{'[', false},
		{'\x1b', false},
		{' ', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.b), func(t *testing.T) {
			if got := isEscTerminator(tt.b); got != tt.want {
				t.Errorf("isEscTerminator(%q) = %v, want %v", tt.b, got, tt.want)
			}
		})
	}
}
