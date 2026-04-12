package tui

import (
	"strings"
	"testing"
)

func TestWrapText_ShortLine(t *testing.T) {
	in := "hello world"
	got := wrapText(in, 80)
	if got != in {
		t.Errorf("expected %q unchanged, got %q", in, got)
	}
}

func TestWrapText_ZeroWidth(t *testing.T) {
	in := "this is a long line that should not be wrapped"
	got := wrapText(in, 0)
	if got != in {
		t.Errorf("zero width should return input unchanged, got %q", got)
	}
}

func TestWrapText_ExactWidth(t *testing.T) {
	// "hello world" is 11 chars; width 11 → no wrap
	got := wrapText("hello world", 11)
	if strings.Contains(got, "\n") {
		t.Errorf("expected no newline for exact-width input, got %q", got)
	}
}

func TestWrapText_Wraps(t *testing.T) {
	got := wrapText("one two three four", 10)
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		if len(line) > 10 {
			t.Errorf("line %q exceeds width 10", line)
		}
	}
	// Reassembled words should match original.
	if strings.Join(strings.Fields(got), " ") != "one two three four" {
		t.Errorf("reassembled text mismatch: %q", got)
	}
}

func TestWrapText_SingleLongWord(t *testing.T) {
	// A single word longer than width must not be broken (no splitting mid-word).
	word := "superlongwordthatexceedswidth"
	got := wrapText(word, 10)
	if got != word {
		t.Errorf("single long word should be kept intact, got %q", got)
	}
}

func TestWrapText_MultipleSpaces(t *testing.T) {
	// strings.Fields collapses multiple spaces — result should still be wrapped correctly.
	got := wrapText("a  b  c", 4)
	// 'a' + ' ' + 'b' = 3 ≤ 4; then adding ' c' = 5 > 4 → wrap before 'c'
	if got != "a b\nc" {
		t.Errorf("expected %q, got %q", "a b\nc", got)
	}
}
