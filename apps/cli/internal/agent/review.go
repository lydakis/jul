package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxAgentStdinBytes = 512 * 1024

func RunReview(ctx context.Context, provider Provider, req ReviewRequest) (ReviewResponse, error) {
	if provider.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, provider.Timeout)
		defer cancel()
	}

	if provider.Bundled {
		return runBundledOpenCode(ctx, provider, req)
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return ReviewResponse{}, err
	}
	mode := provider.Mode
	if mode == "" {
		mode = "stdin"
	}
	if mode == "stdin" && len(payload) > maxAgentStdinBytes {
		mode = "file"
	}

	switch mode {
	case "file":
		return runJSONAgentFile(ctx, provider, req, payload)
	default:
		return runJSONAgentStdin(ctx, provider, payload, req.WorkspacePath)
	}
}

func runJSONAgentStdin(ctx context.Context, provider Provider, payload []byte, workdir string) (ReviewResponse, error) {
	cmdPath, cmdArgs, err := splitCommand(provider.Command)
	if err != nil {
		return ReviewResponse{}, err
	}
	cmd := exec.CommandContext(ctx, cmdPath, cmdArgs...)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(),
		"JUL_AGENT_MODE=stdin",
		"JUL_AGENT_ACTION=review",
		"JUL_AGENT_WORKSPACE="+workdir,
	)
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ReviewResponse{}, fmt.Errorf("agent failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return parseReviewResponse(output)
}

func runJSONAgentFile(ctx context.Context, provider Provider, req ReviewRequest, payload []byte) (ReviewResponse, error) {
	dir := filepath.Dir(req.WorkspacePath)
	if dir == "" {
		dir = "."
	}
	input, err := os.CreateTemp(dir, "jul-agent-input-*.json")
	if err != nil {
		return ReviewResponse{}, err
	}
	defer os.Remove(input.Name())
	if _, err := input.Write(payload); err != nil {
		return ReviewResponse{}, err
	}
	_ = input.Close()

	outputFile, err := os.CreateTemp(dir, "jul-agent-output-*.json")
	if err != nil {
		return ReviewResponse{}, err
	}
	_ = outputFile.Close()
	defer os.Remove(outputFile.Name())

	cmdPath, cmdArgs, err := splitCommand(provider.Command)
	if err != nil {
		return ReviewResponse{}, err
	}
	cmd := exec.CommandContext(ctx, cmdPath, cmdArgs...)
	cmd.Dir = req.WorkspacePath
	cmd.Env = append(os.Environ(),
		"JUL_AGENT_MODE=file",
		"JUL_AGENT_ACTION=review",
		"JUL_AGENT_WORKSPACE="+req.WorkspacePath,
		"JUL_AGENT_INPUT="+input.Name(),
		"JUL_AGENT_OUTPUT="+outputFile.Name(),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ReviewResponse{}, fmt.Errorf("agent failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	data, err := os.ReadFile(outputFile.Name())
	if err != nil {
		return ReviewResponse{}, err
	}
	return parseReviewResponse(data)
}

func runBundledOpenCode(ctx context.Context, provider Provider, req ReviewRequest) (ReviewResponse, error) {
	prompt, attachment := buildOpenCodePrompt(req)
	tempFile, err := writeReviewAttachment(req.WorkspacePath, attachment)
	if err != nil {
		return ReviewResponse{}, err
	}
	defer os.Remove(tempFile)

	cmdPath := provider.Command
	args := []string{"run", "--format", "json", "--file", tempFile, prompt}
	cmd := exec.CommandContext(ctx, cmdPath, args...)
	cmd.Dir = req.WorkspacePath
	cmd.Env = append(os.Environ(),
		"JUL_AGENT_MODE=prompt",
		"JUL_AGENT_ACTION=review",
		"JUL_AGENT_WORKSPACE="+req.WorkspacePath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ReviewResponse{}, fmt.Errorf("opencode failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return parseReviewResponse(output)
}

func splitCommand(command string) (string, []string, error) {
	args, err := parseCommandLine(command)
	if err != nil {
		return "", nil, err
	}
	if len(args) == 0 {
		return "", nil, fmt.Errorf("command required")
	}
	return args[0], args[1:], nil
}

func parseCommandLine(command string) ([]string, error) {
	var args []string
	var buf []rune
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if len(buf) == 0 {
			return
		}
		args = append(args, string(buf))
		buf = buf[:0]
	}

	for _, r := range []rune(command) {
		if escaped {
			buf = append(buf, r)
			escaped = false
			continue
		}
		if r == '\\' && !inSingle {
			escaped = true
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if r == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if !inSingle && !inDouble && (r == ' ' || r == '\t' || r == '\n') {
			flush()
			continue
		}
		buf = append(buf, r)
	}
	if escaped || inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote in command")
	}
	flush()
	return args, nil
}

func parseReviewResponse(data []byte) (ReviewResponse, error) {
	clean := bytes.TrimSpace(data)
	if len(clean) == 0 {
		return ReviewResponse{}, fmt.Errorf("empty agent response")
	}
	var resp ReviewResponse
	if err := json.Unmarshal(clean, &resp); err == nil && resp.Version != 0 {
		return resp, nil
	}
	extracted := extractJSON(clean)
	if err := json.Unmarshal(extracted, &resp); err != nil {
		return ReviewResponse{}, fmt.Errorf("invalid agent response: %w", err)
	}
	return resp, nil
}

func extractJSON(data []byte) []byte {
	trimmed := bytes.TrimSpace(data)
	trimmed = bytes.TrimPrefix(trimmed, []byte("```json"))
	trimmed = bytes.TrimPrefix(trimmed, []byte("```"))
	trimmed = bytes.TrimSuffix(trimmed, []byte("```"))
	trimmed = bytes.TrimSpace(trimmed)
	start := bytes.IndexByte(trimmed, '{')
	if start == -1 {
		return trimmed
	}
	depth := 0
	for i := start; i < len(trimmed); i++ {
		switch trimmed[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return trimmed[start : i+1]
			}
		}
	}
	return trimmed[start:]
}

func buildOpenCodePrompt(req ReviewRequest) (string, string) {
	var buf strings.Builder
	buf.WriteString("You are the Jul internal review agent.\n")
	buf.WriteString("Review the attached context file, make any fixes in the workspace, commit them, and respond with JSON ONLY.\n")
	buf.WriteString("Response schema:\n")
	buf.WriteString("{\"version\":1,\"status\":\"completed\",\"suggestions\":[{\"commit\":\"<sha>\",\"reason\":\"...\",\"description\":\"...\",\"confidence\":0.0}]}\n")
	prompt := buf.String()

	var attachment strings.Builder
	if req.Context.Checkpoint != "" {
		attachment.WriteString("Checkpoint: " + req.Context.Checkpoint + "\n")
	}
	if req.Context.ChangeID != "" {
		attachment.WriteString("Change-Id: " + req.Context.ChangeID + "\n")
	}
	if req.Context.Diff != "" {
		attachment.WriteString("\nDiff:\n")
		attachment.WriteString(req.Context.Diff)
		attachment.WriteString("\n")
	}
	if len(req.Context.Files) > 0 {
		attachment.WriteString("\nFiles:\n")
		for _, file := range req.Context.Files {
			attachment.WriteString("- " + file.Path + "\n")
			if strings.TrimSpace(file.Content) != "" {
				attachment.WriteString("```\n")
				attachment.WriteString(file.Content)
				attachment.WriteString("\n```\n")
			}
		}
	}
	if len(req.Context.CIResults) > 0 {
		attachment.WriteString("\nCI results:\n")
		attachment.Write(req.Context.CIResults)
		attachment.WriteString("\n")
	}
	return prompt, attachment.String()
}

func writeReviewAttachment(workdir, content string) (string, error) {
	dir := workdir
	if strings.TrimSpace(dir) == "" {
		dir = "."
	}
	file, err := os.CreateTemp(dir, "jul-review-*.txt")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		return "", err
	}
	return file.Name(), nil
}
