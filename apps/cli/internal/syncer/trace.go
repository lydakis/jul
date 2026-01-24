package syncer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/ci"
	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/notes"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
)

type TraceOptions struct {
	Prompt          string
	Agent           string
	SessionID       string
	Turn            int
	Force           bool
	Implicit        bool
	UpdateCanonical bool
}

type TraceResult struct {
	TraceSHA     string
	TraceRef     string
	TraceSyncRef string
	CanonicalSHA string
	TraceBase    string
	PromptHash   string
	RemoteName   string
	RemotePushed bool
	Merged       bool
	Skipped      bool
}

func Trace(opts TraceOptions) (TraceResult, error) {
	repoRoot, err := gitutil.RepoTopLevel()
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
	allowCanonical := opts.UpdateCanonical

	remote, rerr := remotesel.Resolve()
	remoteTip := ""
	remoteMissing := false
	if rerr == nil && allowCanonical {
		res.RemoteName = remote.Name
		if err := fetchRef(remote.Name, traceRef); err != nil {
			if isMissingRemoteRef(err) {
				remoteMissing = true
			} else {
				return res, err
			}
		}
		if !remoteMissing {
			if sha, err := gitutil.ResolveRef(traceRef); err == nil {
				remoteTip = strings.TrimSpace(sha)
			}
		} else if sha, err := gitutil.ResolveRef(traceRef); err == nil {
			remoteTip = strings.TrimSpace(sha)
		}
	}

	treeSHA, err := gitutil.DraftTree()
	if err != nil {
		return res, err
	}

	parent := resolveTraceParent(traceSyncRef, traceRef)
	res.TraceBase = parent
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

	traceAttested := false
	if config.TraceRunOnTrace() {
		if err := runTraceCI(traceSHA, repoRoot); err == nil {
			traceAttested = true
		}
	}

	canonical := ""
	existingTip := ""
	if allowCanonical {
		canonical = traceSHA
		existingTip = remoteTip
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
	} else if sha, err := gitutil.ResolveRef(traceRef); err == nil {
		res.CanonicalSHA = strings.TrimSpace(sha)
	}

	if rerr == nil {
		if err := pushRef(remote.Name, traceSHA, traceSyncRef, true); err != nil {
			return res, err
		}
		res.RemotePushed = true
		_ = pushTraceNotes(remote.Name, traceAttested)
		if allowCanonical && canonical != "" {
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

func pushTraceNotes(remoteName string, pushAttestations bool) error {
	if strings.TrimSpace(remoteName) == "" {
		return nil
	}
	traceRef := notes.RefTraces
	if gitutil.RefExists(traceRef) {
		if err := syncNotesRef(remoteName, traceRef); err != nil {
			return err
		}
		if _, err := gitutil.Git("push", remoteName, fmt.Sprintf("%s:%s", traceRef, traceRef)); err != nil {
			return err
		}
	}
	if !pushAttestations {
		return nil
	}
	attRef := notes.RefAttestationsTrace
	if !gitutil.RefExists(attRef) {
		return nil
	}
	if err := syncNotesRef(remoteName, attRef); err != nil {
		return err
	}
	_, err := gitutil.Git("push", remoteName, fmt.Sprintf("%s:%s", attRef, attRef))
	return err
}

func syncNotesRef(remoteName, ref string) error {
	if strings.TrimSpace(remoteName) == "" || strings.TrimSpace(ref) == "" {
		return nil
	}
	tmpRef := fmt.Sprintf("refs/jul/tmp/notes/%d", time.Now().UnixNano())
	if _, err := gitutil.Git("fetch", remoteName, "+"+ref+":"+tmpRef); err != nil {
		if isMissingRemoteRef(err) {
			return nil
		}
		return err
	}
	if _, err := gitutil.Git("notes", "--ref", ref, "merge", tmpRef); err != nil {
		_, _ = gitutil.Git("notes", "--ref", ref, "merge", "--abort")
		_, _ = gitutil.Git("update-ref", "-d", tmpRef)
		return err
	}
	_, _ = gitutil.Git("update-ref", "-d", tmpRef)
	return nil
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]+`),
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|pwd)\s*[:=]\s*\S+`),
}

func runTraceCI(traceSHA, repoRoot string) error {
	cmds := resolveTraceCommands(repoRoot)
	if len(cmds) == 0 {
		return nil
	}
	result, err := ci.RunCommands(cmds, repoRoot)
	if err != nil {
		return err
	}
	signals, err := json.Marshal(result)
	if err != nil {
		return err
	}
	att := client.Attestation{
		CommitSHA:   traceSHA,
		Type:        "trace",
		Status:      result.Status,
		StartedAt:   result.StartedAt,
		FinishedAt:  result.FinishedAt,
		SignalsJSON: string(signals),
	}
	_, err = metadata.WriteTraceAttestation(att)
	return err
}

func resolveTraceCommands(repoRoot string) []string {
	checks := config.TraceChecks()
	if len(checks) == 0 {
		return nil
	}
	hasGoMod := fileExists(filepath.Join(repoRoot, "go.mod"))
	hasGoWork := fileExists(filepath.Join(repoRoot, "go.work"))
	cmds := make([]string, 0, len(checks))
	for _, check := range checks {
		check = strings.TrimSpace(check)
		if check == "" {
			continue
		}
		cmd := traceCheckCommand(check, hasGoMod || hasGoWork)
		if cmd == "" {
			continue
		}
		cmds = append(cmds, cmd)
	}
	return cmds
}

func traceCheckCommand(check string, hasGo bool) string {
	lower := strings.ToLower(strings.TrimSpace(check))
	if strings.Contains(check, " ") || strings.Contains(check, "/") {
		return check
	}
	switch lower {
	case "lint":
		if hasGo {
			return "go vet ./..."
		}
	case "typecheck":
		if hasGo {
			return "go test ./... -run TestDoesNotExist"
		}
	default:
		return check
	}
	return ""
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
