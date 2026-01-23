package syncer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
)

type TraceOptions struct {
	Prompt    string
	Agent     string
	SessionID string
	Turn      int
	Force     bool
	Implicit  bool
}

type TraceResult struct {
	TraceSHA     string
	TraceRef     string
	TraceSyncRef string
	CanonicalSHA string
	PromptHash   string
	RemoteName   string
	RemotePushed bool
	Merged       bool
	Skipped      bool
}

func Trace(opts TraceOptions) (TraceResult, error) {
	_, err := gitutil.RepoTopLevel()
	if err != nil {
		return TraceResult{}, err
	}
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		return TraceResult{}, err
	}

	traceRef := fmt.Sprintf("refs/jul/traces/%s/%s", user, workspace)
	traceSyncRef := fmt.Sprintf("refs/jul/trace-sync/%s/%s/%s", user, deviceID, workspace)

	res := TraceResult{
		TraceRef:     traceRef,
		TraceSyncRef: traceSyncRef,
	}

	remote, rerr := remotesel.Resolve()
	remoteTip := ""
	if rerr == nil {
		res.RemoteName = remote.Name
		if err := fetchRef(remote.Name, traceRef); err != nil {
			if !isMissingRemoteRef(err) {
				return res, err
			}
		}
		if sha, err := gitutil.ResolveRef(traceRef); err == nil {
			remoteTip = strings.TrimSpace(sha)
		}
	}

	treeSHA, err := gitutil.DraftTree()
	if err != nil {
		return res, err
	}

	parent := resolveTraceParent(traceSyncRef, traceRef)
	if !opts.Force {
		if parent != "" {
			if parentTree, err := gitutil.TreeOf(parent); err == nil && parentTree == treeSHA {
				res.TraceSHA = parent
				res.CanonicalSHA = parent
				res.Skipped = true
				if !gitutil.RefExists(traceSyncRef) {
					_ = gitutil.UpdateRef(traceSyncRef, parent)
				}
				return res, nil
			}
		}
	}

	message := traceMessage(opts.Agent)
	traceSHA, err := gitutil.CommitTreeWithParents(treeSHA, []string{parent}, message)
	if err != nil {
		return res, err
	}
	res.TraceSHA = traceSHA

	if err := gitutil.UpdateRef(traceSyncRef, traceSHA); err != nil {
		return res, err
	}

	prompt := strings.TrimSpace(opts.Prompt)
	var promptHash string
	if prompt != "" {
		hash := sha256.Sum256([]byte(prompt))
		promptHash = "sha256:" + hex.EncodeToString(hash[:])
	}
	res.PromptHash = promptHash

	if prompt != "" {
		_ = metadata.WriteTracePrompt(traceSHA, prompt)
		_ = metadata.WriteTraceSummary(traceSHA, summarizePrompt(prompt))
	}

	note := metadata.TraceNote{
		TraceSHA:  traceSHA,
		Agent:     strings.TrimSpace(opts.Agent),
		SessionID: strings.TrimSpace(opts.SessionID),
		Turn:      opts.Turn,
		Device:    deviceID,
		CreatedAt: time.Now().UTC(),
	}
	if prompt != "" {
		if config.TraceSyncPromptHash() {
			note.PromptHash = promptHash
		}
		if config.TraceSyncPromptSummary() {
			summary := summarizePrompt(prompt)
			if summary != "" {
				note.PromptSummary = scrubSecrets(summary)
			}
		}
		if config.TraceSyncPromptFull() {
			note.PromptFull = prompt
		}
	}
	if err := metadata.WriteTrace(note); err != nil {
		return res, err
	}

	canonical := traceSHA
	existingTip := remoteTip
	if existingTip == "" {
		if sha, err := gitutil.ResolveRef(traceRef); err == nil {
			existingTip = strings.TrimSpace(sha)
		}
	}
	if existingTip != "" && existingTip != traceSHA {
		switch {
		case gitutil.IsAncestor(existingTip, traceSHA):
			canonical = traceSHA
		case gitutil.IsAncestor(traceSHA, existingTip):
			canonical = existingTip
		default:
			mergeMessage := "[trace] merge"
			mergeSHA, err := gitutil.CommitTreeWithParents(treeSHA, []string{existingTip, traceSHA}, mergeMessage)
			if err != nil {
				return res, err
			}
			canonical = mergeSHA
			res.Merged = true
		}
	}
	res.CanonicalSHA = canonical
	if canonical != "" {
		if err := gitutil.UpdateRef(traceRef, canonical); err != nil {
			return res, err
		}
	}

	if rerr == nil {
		if err := pushRef(remote.Name, traceSHA, traceSyncRef, true); err != nil {
			return res, err
		}
		res.RemotePushed = true
		if canonical != "" {
			if err := pushWorkspace(remote.Name, canonical, traceRef, remoteTip); err != nil {
				return res, err
			}
		}
	}

	return res, nil
}

func resolveTraceParent(traceSyncRef, traceRef string) string {
	if gitutil.RefExists(traceSyncRef) {
		if sha, err := gitutil.ResolveRef(traceSyncRef); err == nil {
			return strings.TrimSpace(sha)
		}
	}
	if gitutil.RefExists(traceRef) {
		if sha, err := gitutil.ResolveRef(traceRef); err == nil {
			return strings.TrimSpace(sha)
		}
	}
	return ""
}

func traceMessage(agent string) string {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return "[trace]"
	}
	return fmt.Sprintf("[trace] agent:%s", agent)
}

func summarizePrompt(prompt string) string {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return ""
	}
	if idx := strings.IndexRune(trimmed, '\n'); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	max := 120
	if len(trimmed) > max {
		trimmed = trimmed[:max] + "..."
	}
	return trimmed
}

func scrubSecrets(summary string) string {
	out := summary
	for _, re := range secretPatterns {
		out = re.ReplaceAllString(out, "[redacted]")
	}
	return out
}

func isMissingRemoteRef(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "couldn't find remote ref") || strings.Contains(msg, "remote ref does not exist")
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`),
	regexp.MustCompile(`(?i)bearer\\s+[A-Za-z0-9._-]+`),
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|pwd)\\s*[:=]\\s*\\S+`),
}
