package cli

import "testing"

func TestParseBlamePorcelainSupportsGroupedLines(t *testing.T) {
	input := "aaaaaaaa 1 1 2\n" +
		"author Alice\n" +
		"summary first\n" +
		"\tline one\n" +
		"aaaaaaaa 2 2\n" +
		"\tline two\n" +
		"bbbbbbbb 3 3 1\n" +
		"author Bob\n" +
		"summary second\n" +
		"\tline three\n"
	lines := parseBlamePorcelain(input)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0].Line != 1 || lines[0].Content != "line one" {
		t.Fatalf("unexpected first line: %+v", lines[0])
	}
	if lines[1].Line != 2 || lines[1].Content != "line two" {
		t.Fatalf("unexpected second line: %+v", lines[1])
	}
	if lines[2].Line != 3 || lines[2].Content != "line three" {
		t.Fatalf("unexpected third line: %+v", lines[2])
	}
}
