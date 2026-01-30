package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/metadata"
)

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

func TestBlameSkipsMergeAndRestackTraces(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")

	writeFilePath(t, repo, "app.txt", "one\n")
	runGitCmd(t, repo, "add", "app.txt")
	runGitCmd(t, repo, "commit", "-m", "base")
	baseSHA := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))

	writeFilePath(t, repo, "app.txt", "two\n")
	runGitCmd(t, repo, "add", "app.txt")
	runGitCmd(t, repo, "commit", "-m", "change")
	changeSHA := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("HOME", filepath.Join(repo, "home"))

	if err := metadata.WriteTrace(metadata.TraceNote{TraceSHA: changeSHA, TraceType: "merge"}); err != nil {
		t.Fatalf("write trace note failed: %v", err)
	}

	attrib := resolveTraceAttribution(repo, changeSHA, map[string]string{})
	if strings.TrimSpace(attrib) != baseSHA {
		t.Fatalf("expected blame to skip merge trace and use base %s, got %s", baseSHA, attrib)
	}
}
