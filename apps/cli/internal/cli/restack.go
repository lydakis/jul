package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/agent"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/output"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
	"github.com/lydakis/jul/cli/internal/restack"
	"github.com/lydakis/jul/cli/internal/workspace"
)

func runWorkspaceRestack(args []string) int {
	fs, jsonOut := newFlagSet("ws restack")
	onto := fs.String("onto", "", "Retarget base to ref (e.g. main)")
	_ = fs.Parse(args)

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "restack_repo_failed", fmt.Sprintf("failed to locate repo root: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to locate repo root: %v\n", err)
		}
		return 1
	}
	user, ws := workspaceParts()
	if ws == "" {
		ws = "@"
	}

	cfg, _, err := workspace.ReadConfig(repoRoot, ws)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "restack_config_failed", fmt.Sprintf("failed to read workspace config: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to read workspace config: %v\n", err)
		}
		return 1
	}
	baseRef := strings.TrimSpace(cfg.BaseRef)
	if ontoVal := strings.TrimSpace(*onto); ontoVal != "" {
		baseRef, err = normalizeBaseRef(repoRoot, ontoVal)
		if err != nil {
			if *jsonOut {
				_ = output.EncodeError(os.Stdout, "restack_invalid_onto", fmt.Sprintf("invalid --onto: %v", err), nil)
			} else {
				fmt.Fprintf(os.Stderr, "invalid --onto: %v\n", err)
			}
			return 1
		}
	}
	if baseRef == "" {
		baseRef = detectBaseRef(repoRoot)
	}
	if baseRef == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "restack_base_missing", "base ref not found; use --onto to specify", nil)
		} else {
			fmt.Fprintln(os.Stderr, "base ref not found; use --onto to specify")
		}
		return 1
	}

	baseTip, err := resolveBaseTip(repoRoot, baseRef)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "restack_base_resolve_failed", fmt.Sprintf("failed to resolve base ref: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to resolve base ref: %v\n", err)
		}
		return 1
	}
	if baseTip == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "restack_base_tip_missing", "base tip not found; checkpoint required on base workspace", nil)
		} else {
			fmt.Fprintln(os.Stderr, "base tip not found; checkpoint required on base workspace")
		}
		return 1
	}

	res, err := restack.Run(restack.Options{
		RepoRoot:  repoRoot,
		User:      user,
		Workspace: ws,
		BaseRef:   baseRef,
		BaseTip:   baseTip,
	})
	if err != nil {
		if errors.Is(err, agent.ErrMergeInProgress) {
			if *jsonOut {
				_ = output.EncodeError(os.Stdout, "restack_merge_in_progress", "restack blocked: merge in progress; run 'jul merge' first", nil)
			} else {
				fmt.Fprintln(os.Stderr, "restack blocked: merge in progress; run 'jul merge' first")
			}
			return 1
		}
		var conflict restack.ConflictError
		if errors.As(err, &conflict) {
			if *jsonOut {
				_ = output.EncodeError(os.Stdout, "restack_conflict", "restack conflict; run 'jul merge' to resolve", nil)
			} else {
				fmt.Fprintf(os.Stderr, "Restack conflict on checkpoint %s\n", strings.TrimSpace(conflict.CheckpointSHA))
				if len(conflict.Conflicts) > 0 {
					fmt.Fprintln(os.Stderr, "Conflicts in:")
					for _, file := range conflict.Conflicts {
						fmt.Fprintf(os.Stderr, "  - %s\n", file)
					}
				}
				fmt.Fprintln(os.Stderr, "Run 'jul merge' to resolve.")
			}
			return 1
		}
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "restack_failed", fmt.Sprintf("restack failed: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "restack failed: %v\n", err)
		}
		return 1
	}
	out := workspaceActionOutput{
		Status:    "ok",
		Action:    "restack",
		Workspace: user + "/" + ws,
		Message:   fmt.Sprintf("Restacked %d checkpoints onto %s", len(res.NewCheckpoints), strings.TrimSpace(baseTip)),
	}
	if *jsonOut {
		if code := writeJSON(out); code != 0 {
			return code
		}
	} else {
		renderWorkspaceAction(out)
	}

	if len(res.NewCheckpoints) > 0 {
		remote, rerr := remotesel.Resolve()
		if rerr == nil && strings.TrimSpace(remote.Name) != "" {
			ref := changeRef(res.ChangeID)
			remoteTip, _ := remoteRefTip(remote.Name, ref)
			if err := pushWorkspace(remote.Name, res.NewCheckpoints[len(res.NewCheckpoints)-1], ref, remoteTip); err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "restack_push_failed", fmt.Sprintf("failed to push change ref: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "failed to push change ref: %v\n", err)
				}
				return 1
			}
			anchor := anchorRef(res.ChangeID)
			anchorTip, _ := remoteRefTip(remote.Name, anchor)
			if anchorTip == "" {
				if err := pushRef(remote.Name, res.NewCheckpoints[0], anchor, false); err != nil {
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, "restack_push_failed", fmt.Sprintf("failed to push anchor ref: %v", err), nil)
					} else {
						fmt.Fprintf(os.Stderr, "failed to push anchor ref: %v\n", err)
					}
					return 1
				}
			}
		}
	}
	return 0
}

func isEmptyCherryPick(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "cherry-pick") {
		return false
	}
	if strings.Contains(msg, "previous cherry-pick is empty") {
		return true
	}
	if strings.Contains(msg, "patch is empty") {
		return true
	}
	if strings.Contains(msg, "nothing to commit") {
		return true
	}
	return false
}

func checkpointChain(latestSHA, changeID string) ([]string, error) {
	chain := []string{}
	sha := strings.TrimSpace(latestSHA)
	for sha != "" {
		chain = append(chain, sha)
		parent, err := gitutil.ParentOf(sha)
		if err != nil {
			break
		}
		parent = strings.TrimSpace(parent)
		if parent == "" {
			break
		}
		msg, err := gitutil.CommitMessage(parent)
		if err != nil {
			break
		}
		if gitutil.ExtractChangeID(msg) != changeID {
			break
		}
		sha = parent
	}
	// reverse to oldest -> newest
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

func normalizeBaseRef(repoRoot, value string) (string, error) {
	val := strings.TrimSpace(value)
	if val == "" {
		return "", fmt.Errorf("base ref required")
	}
	if strings.HasPrefix(val, "refs/") {
		return val, nil
	}
	ref := "refs/heads/" + val
	if refExists(repoRoot, ref) {
		return ref, nil
	}
	return ref, nil
}

func resolveBaseTip(repoRoot, baseRef string) (string, error) {
	if strings.TrimSpace(baseRef) == "" {
		return "", fmt.Errorf("base ref required")
	}
	sha, err := gitutil.Git("-C", repoRoot, "rev-parse", baseRef)
	if err != nil {
		return "", err
	}
	sha = strings.TrimSpace(sha)
	if strings.HasPrefix(baseRef, "refs/jul/workspaces/") {
		parent, err := gitutil.ParentOf(sha)
		if err == nil && strings.TrimSpace(parent) != "" {
			return strings.TrimSpace(parent), nil
		}
		return "", fmt.Errorf("base workspace has no checkpoint")
	}
	if strings.HasPrefix(baseRef, "refs/jul/changes/") {
		if sha == "" {
			return "", fmt.Errorf("change ref missing")
		}
		return sha, nil
	}
	return sha, nil
}

func updateWorktreeLocal(repoRoot, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("ref required for worktree update")
	}
	if _, err := gitutil.Git("-C", repoRoot, "read-tree", "--reset", "-u", ref); err != nil {
		return err
	}
	_, err := gitutil.Git("-C", repoRoot, "clean", "-fd", "--exclude=.jul")
	return err
}
