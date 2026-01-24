package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPromptTemplateReplaces(t *testing.T) {
	args := []string{"run", "--prompt", "$PROMPT", "--file", "$ATTACHMENT"}
	prompt := "Review this"
	attachment := "/tmp/review.txt"

	out, replaced := applyPromptTemplate(args, prompt, attachment)
	if !replaced {
		t.Fatalf("expected replacement")
	}
	if out[2] != prompt {
		t.Fatalf("expected prompt replacement, got %q", out[2])
	}
	if out[4] != attachment {
		t.Fatalf("expected attachment replacement, got %q", out[4])
	}
}

func TestApplyPromptTemplateNoReplacement(t *testing.T) {
	args := []string{"run", "--mode", "json"}
	out, replaced := applyPromptTemplate(args, "prompt", "/tmp/file")
	if replaced {
		t.Fatalf("did not expect replacement")
	}
	if len(out) != len(args) {
		t.Fatalf("expected output length %d, got %d", len(args), len(out))
	}
}

func TestParseCommandLinePreservesQuotes(t *testing.T) {
	cmd := `"C:\Program Files\Agent\agent.exe" --flag "some value" 'other value'`
	args, err := parseCommandLine(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d (%v)", len(args), args)
	}
	if args[0] != `C:\Program Files\Agent\agent.exe` {
		t.Fatalf("unexpected command: %q", args[0])
	}
	if args[2] != "some value" || args[3] != "other value" {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestReviewPromptKeepsDiffInAttachment(t *testing.T) {
	req := ReviewRequest{
		Context: ReviewContext{
			Checkpoint: "abc123",
			ChangeID:   "Iabc123",
			Diff:       "diff --git a/file b/file",
			Files: []ReviewFile{
				{Path: "file.txt", Content: "content"},
			},
		},
	}
	attachment := buildReviewAttachment(req)
	prompt := buildReviewPrompt("review", "/tmp/review.txt")
	if !strings.Contains(attachment, "diff --git") || !strings.Contains(attachment, "file.txt") {
		t.Fatalf("expected diff and file content in attachment")
	}
	if strings.Contains(prompt, "diff --git") || strings.Contains(prompt, "file.txt") {
		t.Fatalf("did not expect diff or file content in prompt")
	}
}

func TestWriteReviewAttachmentOutsideWorktree(t *testing.T) {
	tmp := t.TempDir()
	workdir := filepath.Join(tmp, "worktree")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("failed to create workdir: %v", err)
	}
	path, err := writeReviewAttachment(workdir, "content")
	if err != nil {
		t.Fatalf("failed to write attachment: %v", err)
	}
	if strings.HasPrefix(path, workdir+string(filepath.Separator)) {
		t.Fatalf("expected attachment outside worktree, got %s", path)
	}
}
