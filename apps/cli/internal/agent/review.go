package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const maxAgentStdinBytes = 512 * 1024

func RunReview(ctx context.Context, provider Provider, req ReviewRequest) (ReviewResponse, error) {
	return RunReviewWithStream(ctx, provider, req, nil)
}

func RunReviewWithStream(ctx context.Context, provider Provider, req ReviewRequest, stream io.Writer) (ReviewResponse, error) {
	if provider.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, provider.Timeout)
		defer cancel()
	}

	if provider.Bundled {
		return runBundledOpenCode(ctx, provider, req, stream)
	}
	headless := provider.HeadlessFor("review")
	if headless != "" || strings.EqualFold(provider.Mode, "prompt") {
		return runPromptAgent(ctx, provider, req, headless, stream)
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
		return runJSONAgentFile(ctx, provider, req, payload, stream)
	default:
		return runJSONAgentStdin(ctx, provider, payload, req.WorkspacePath, stream)
	}
}

func runJSONAgentStdin(ctx context.Context, provider Provider, payload []byte, workdir string, stream io.Writer) (ReviewResponse, error) {
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
	output, err := runCommandWithStream(cmd, stream)
	if err != nil {
		return ReviewResponse{}, fmt.Errorf("agent failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return parseReviewResponse(output)
}

func runJSONAgentFile(ctx context.Context, provider Provider, req ReviewRequest, payload []byte, stream io.Writer) (ReviewResponse, error) {
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
	output, err := runCommandWithStream(cmd, stream)
	if err != nil {
		return ReviewResponse{}, fmt.Errorf("agent failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	data, err := os.ReadFile(outputFile.Name())
	if err != nil {
		return ReviewResponse{}, err
	}
	return parseReviewResponse(data)
}

func runBundledOpenCode(ctx context.Context, provider Provider, req ReviewRequest, stream io.Writer) (ReviewResponse, error) {
	attachment := buildReviewAttachment(req)
	tempFile, err := writeReviewAttachment(req.WorkspacePath, attachment)
	if err != nil {
		return ReviewResponse{}, err
	}
	defer os.Remove(tempFile)

	prompt := buildReviewPrompt(req.Action, tempFile)
	cmdPath := provider.Command
	args := []string{"run", "--format", "json", "--file", tempFile, "--", prompt}
	cmd := exec.CommandContext(ctx, cmdPath, args...)
	cmd.Dir = req.WorkspacePath
	cmd.Env = append(os.Environ(),
		"JUL_AGENT_MODE=prompt",
		"JUL_AGENT_ACTION="+req.Action,
		"JUL_AGENT_WORKSPACE="+req.WorkspacePath,
	)
	output, err := runCommandWithStream(cmd, stream)
	if err != nil {
		return ReviewResponse{}, fmt.Errorf("opencode failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return parseReviewResponse(output)
}

func runPromptAgent(ctx context.Context, provider Provider, req ReviewRequest, headless string, stream io.Writer) (ReviewResponse, error) {
	attachment := buildReviewAttachment(req)
	tempFile, err := writeReviewAttachment(req.WorkspacePath, attachment)
	if err != nil {
		return ReviewResponse{}, err
	}
	defer os.Remove(tempFile)

	prompt := buildReviewPrompt(req.Action, tempFile)
	command := strings.TrimSpace(headless)
	if command == "" {
		command = provider.Command
	}
	cmdPath, cmdArgs, err := splitCommand(command)
	if err != nil {
		return ReviewResponse{}, err
	}
	cmdArgs, replaced := applyPromptTemplate(cmdArgs, prompt, tempFile)
	if !replaced {
		cmdArgs = append(cmdArgs, prompt)
	}
	cmd := exec.CommandContext(ctx, cmdPath, cmdArgs...)
	cmd.Dir = req.WorkspacePath
	cmd.Env = append(os.Environ(),
		"JUL_AGENT_MODE=prompt",
		"JUL_AGENT_ACTION="+req.Action,
		"JUL_AGENT_WORKSPACE="+req.WorkspacePath,
		"JUL_AGENT_INPUT="+tempFile,
		"JUL_AGENT_PROMPT="+prompt,
	)
	output, err := runCommandWithStream(cmd, stream)
	if err != nil {
		return ReviewResponse{}, fmt.Errorf("agent failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return parseReviewResponse(output)
}

type streamSink struct {
	w  io.Writer
	mu sync.Mutex
}

func (s *streamSink) Println(msg string) {
	if s == nil || s.w == nil || strings.TrimSpace(msg) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintln(s.w, msg)
}

func runCommandWithStream(cmd *exec.Cmd, stream io.Writer) ([]byte, error) {
	if stream == nil {
		return cmd.CombinedOutput()
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	sink := &streamSink{w: stream}
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		streamAgentOutput(stdout, &stdoutBuf, sink, true)
	}()
	go func() {
		defer wg.Done()
		streamAgentOutput(stderr, &stderrBuf, sink, false)
	}()
	err = cmd.Wait()
	wg.Wait()
	output := append(stdoutBuf.Bytes(), stderrBuf.Bytes()...)
	return output, err
}

func streamAgentOutput(r io.Reader, buf *bytes.Buffer, sink *streamSink, parseJSON bool) {
	reader := bufio.NewReader(r)
	var lineBuf bytes.Buffer
	flush := func() {
		if lineBuf.Len() == 0 {
			return
		}
		line := lineBuf.String()
		lineBuf.Reset()
		if sink != nil {
			if msg := formatAgentStreamLine(line, parseJSON); msg != "" {
				sink.Println("agent: " + msg)
			}
		}
	}

	for {
		chunk, err := reader.ReadSlice('\n')
		if len(chunk) > 0 {
			buf.Write(chunk)
			lineBuf.Write(chunk)
			if chunk[len(chunk)-1] == '\n' {
				flush()
			}
		}
		if err != nil {
			if err == bufio.ErrBufferFull {
				continue
			}
			if err == io.EOF {
				flush()
				return
			}
			flush()
			return
		}
	}
}

func formatAgentStreamLine(line string, parseJSON bool) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	if parseJSON && strings.HasPrefix(trimmed, "{") {
		if msg := summarizeAgentJSON([]byte(trimmed)); msg != "" {
			return msg
		}
	}
	return trimmed
}

func summarizeAgentJSON(data []byte) string {
	var resp ReviewResponse
	if err := json.Unmarshal(data, &resp); err == nil && resp.Version != 0 {
		status := strings.TrimSpace(resp.Status)
		if status == "" {
			status = "completed"
		}
		return fmt.Sprintf("completed (%s)", status)
	}
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return ""
	}
	if value, ok := event["event"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := event["type"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := event["status"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := event["message"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := event["name"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
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

func applyPromptTemplate(args []string, prompt, attachment string) ([]string, bool) {
	out := make([]string, len(args))
	replaced := false
	for i, arg := range args {
		next := strings.ReplaceAll(arg, "$PROMPT", prompt)
		next = strings.ReplaceAll(next, "$ATTACHMENT", attachment)
		if next != arg {
			replaced = true
		}
		out[i] = next
	}
	return out, replaced
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

	runes := []rune(command)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if escaped {
			buf = append(buf, r)
			escaped = false
			continue
		}
		if r == '\\' && !inSingle {
			var next rune
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			if inDouble {
				if next == '"' || next == '\\' || next == '$' || next == '\n' {
					escaped = true
					continue
				}
			} else {
				if next == ' ' || next == '\t' || next == '\n' || next == '"' || next == '\\' {
					escaped = true
					continue
				}
			}
			buf = append(buf, r)
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
	var last ReviewResponse
	found := false
	for _, line := range bytes.Split(clean, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var candidate ReviewResponse
		if err := json.Unmarshal(line, &candidate); err == nil && candidate.Version != 0 {
			last = candidate
			found = true
			continue
		}
		extracted := extractJSON(line)
		if err := json.Unmarshal(extracted, &candidate); err == nil && candidate.Version != 0 {
			last = candidate
			found = true
		}
	}
	if found {
		return last, nil
	}
	if text, ok, err := extractOpenCodeText(clean); ok {
		if err != nil {
			return ReviewResponse{}, err
		}
		textBytes := bytes.TrimSpace([]byte(text))
		if len(textBytes) == 0 {
			return ReviewResponse{}, fmt.Errorf("invalid agent response: empty text output")
		}
		if err := json.Unmarshal(textBytes, &resp); err == nil && resp.Version != 0 {
			return resp, nil
		}
		for _, line := range bytes.Split(textBytes, []byte("\n")) {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			var candidate ReviewResponse
			if err := json.Unmarshal(line, &candidate); err == nil && candidate.Version != 0 {
				last = candidate
				found = true
				continue
			}
			extracted := extractJSON(line)
			if err := json.Unmarshal(extracted, &candidate); err == nil && candidate.Version != 0 {
				last = candidate
				found = true
			}
		}
		if found {
			return last, nil
		}
		extracted := extractJSON(textBytes)
		if err := json.Unmarshal(extracted, &resp); err != nil {
			return ReviewResponse{}, fmt.Errorf("invalid agent response: %w", err)
		}
		if resp.Version == 0 {
			return ReviewResponse{}, fmt.Errorf("invalid agent response: missing version")
		}
		return resp, nil
	}
	extracted := extractJSON(clean)
	if err := json.Unmarshal(extracted, &resp); err != nil {
		return ReviewResponse{}, fmt.Errorf("invalid agent response: %w", err)
	}
	if resp.Version == 0 {
		return ReviewResponse{}, fmt.Errorf("invalid agent response: missing version")
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

type openCodeEvent struct {
	Type string `json:"type"`
	Part struct {
		Text string `json:"text"`
	} `json:"part"`
	Error *struct {
		Name string `json:"name"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	} `json:"error"`
}

func extractOpenCodeText(data []byte) (string, bool, error) {
	var text strings.Builder
	var sawEvent bool
	var errMsg string
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var event openCodeEvent
		if err := json.Unmarshal(line, &event); err != nil || strings.TrimSpace(event.Type) == "" {
			continue
		}
		sawEvent = true
		if event.Type == "text" {
			if chunk := event.Part.Text; chunk != "" {
				text.WriteString(chunk)
			}
		}
		if event.Type == "error" && errMsg == "" && event.Error != nil {
			msg := strings.TrimSpace(event.Error.Data.Message)
			if msg == "" {
				msg = strings.TrimSpace(event.Error.Name)
			}
			if msg != "" {
				errMsg = msg
			}
		}
	}
	if !sawEvent {
		return "", false, nil
	}
	if text.Len() == 0 && errMsg != "" {
		return "", true, fmt.Errorf("agent error: %s", errMsg)
	}
	return text.String(), true, nil
}

func buildReviewPrompt(action, attachmentPath string) string {
	var buf strings.Builder
	action = strings.TrimSpace(action)
	if action == "" {
		action = "review_suggest"
	}
	switch action {
	case "resolve_conflict":
		buf.WriteString("You are the Jul internal merge agent.\n")
	case "generate_message":
		buf.WriteString("You are the Jul internal checkpoint message agent.\n")
	case "review_summary":
		buf.WriteString("You are the Jul internal review agent.\n")
	default:
		buf.WriteString("You are the Jul internal review agent.\n")
	}
	if strings.TrimSpace(attachmentPath) != "" {
		switch action {
		case "resolve_conflict":
			buf.WriteString("Resolve merge conflicts using the context file at ")
			buf.WriteString(attachmentPath)
			buf.WriteString(". Fix conflicts in the workspace, ensure it builds, commit the resolution, and respond with JSON ONLY.\n")
		case "generate_message":
			buf.WriteString("Generate a concise checkpoint commit message using the context file at ")
			buf.WriteString(attachmentPath)
			buf.WriteString(". Do not modify the workspace. Respond with JSON ONLY. Put the full commit message text in summary. Do not include Change-Id or Trace trailers.\n")
		case "review_summary":
			buf.WriteString("Review the context file at ")
			buf.WriteString(attachmentPath)
			buf.WriteString(". Do not modify the workspace. Respond with JSON ONLY containing a concise summary of findings and recommendations.\n")
		default:
			buf.WriteString("Review the context file at ")
			buf.WriteString(attachmentPath)
			buf.WriteString(", make fixes in the workspace, commit them, and respond with JSON ONLY.\n")
		}
	} else {
		switch action {
		case "resolve_conflict":
			buf.WriteString("Resolve merge conflicts using the attached context file. Fix conflicts in the workspace, ensure it builds, commit the resolution, and respond with JSON ONLY.\n")
		case "generate_message":
			buf.WriteString("Generate a concise checkpoint commit message using the attached context file. Do not modify the workspace. Respond with JSON ONLY. Put the full commit message text in summary. Do not include Change-Id or Trace trailers.\n")
		case "review_summary":
			buf.WriteString("Review the attached context file. Do not modify the workspace. Respond with JSON ONLY containing a concise summary of findings and recommendations.\n")
		default:
			buf.WriteString("Review the attached context file, make fixes in the workspace, commit them, and respond with JSON ONLY.\n")
		}
	}
	buf.WriteString("Response schema:\n")
	switch action {
	case "generate_message":
		buf.WriteString("{\"version\":1,\"status\":\"completed\",\"summary\":\"feat: ...\"}\n")
	case "review_summary":
		buf.WriteString("{\"version\":1,\"status\":\"completed\",\"summary\":\"...\"}\n")
	default:
		buf.WriteString("{\"version\":1,\"status\":\"completed\",\"suggestions\":[{\"commit\":\"<sha>\",\"reason\":\"...\",\"description\":\"...\",\"confidence\":0.0}]}\n")
	}
	return buf.String()
}

func buildReviewAttachment(req ReviewRequest) string {
	var attachment strings.Builder
	if req.Context.Checkpoint != "" {
		attachment.WriteString("Checkpoint: " + req.Context.Checkpoint + "\n")
	}
	if req.Context.ChangeID != "" {
		attachment.WriteString("Change-Id: " + req.Context.ChangeID + "\n")
	}
	if len(req.Context.Conflicts) > 0 {
		attachment.WriteString("\nConflicts:\n")
		for _, conflict := range req.Context.Conflicts {
			if strings.TrimSpace(conflict) == "" {
				continue
			}
			attachment.WriteString("- " + strings.TrimSpace(conflict) + "\n")
		}
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
	if strings.TrimSpace(req.Context.PriorSummary) != "" {
		attachment.WriteString("\nPrior review summary:\n")
		attachment.WriteString(strings.TrimSpace(req.Context.PriorSummary))
		attachment.WriteString("\n")
	}
	return attachment.String()
}

func writeReviewAttachment(workdir, content string) (string, error) {
	dir := os.TempDir()
	if strings.TrimSpace(workdir) != "" {
		parent := filepath.Dir(workdir)
		if parent != "" && parent != "." && parent != string(filepath.Separator) {
			dir = filepath.Join(parent, "attachments")
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
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
