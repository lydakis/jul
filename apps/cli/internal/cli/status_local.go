package cli

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
)

func buildLocalStatus() (output.Status, error) {
	info, err := gitutil.CurrentCommit()
	if err != nil {
		info, err = fallbackCommitInfo()
		if err != nil {
			return output.Status{}, err
		}
	}
	repoName := config.RepoName()
	if repoName != "" {
		info.RepoName = repoName
	}

	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	wsID := config.WorkspaceID()

	draftSHA := ""
	if ref, err := syncRef(user, workspace); err == nil && gitutil.RefExists(ref) {
		if sha, err := gitutil.ResolveRef(ref); err == nil {
			draftSHA = sha
		}
	}
	if draftSHA == "" {
		ref := workspaceRef(user, workspace)
		if gitutil.RefExists(ref) {
			if sha, err := gitutil.ResolveRef(ref); err == nil {
				draftSHA = sha
			}
		}
	}
	if draftSHA == "" {
		draftSHA = info.SHA
	}

	var checkpoint *output.CheckpointStatus
	var att *client.Attestation
	last, err := latestCheckpoint()
	if err != nil {
		return output.Status{}, err
	}
	if last != nil {
		checkpoint = &output.CheckpointStatus{
			CommitSHA: last.SHA,
			Message:   firstLine(last.Message),
			Author:    last.Author,
			When:      last.When.Format("2006-01-02 15:04:05"),
			ChangeID:  last.ChangeID,
		}
		att, _ = metadata.GetAttestation(last.SHA)
	}
	if att == nil {
		att, _ = metadata.GetAttestation(info.SHA)
	}

	changeID := info.ChangeID
	if checkpoint != nil && checkpoint.ChangeID != "" {
		changeID = checkpoint.ChangeID
	}
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(info.SHA)
	}

	suggestions, err := metadata.ListSuggestions(changeID, "pending", 1000)
	if err != nil {
		return output.Status{}, err
	}

	status := output.Status{
		WorkspaceID:        wsID,
		Repo:               info.RepoName,
		Branch:             info.Branch,
		DraftSHA:           draftSHA,
		ChangeID:           changeID,
		SyncStatus:         "local",
		LastCheckpoint:     checkpoint,
		SuggestionsPending: len(suggestions),
	}
	if att != nil {
		status.AttestationStatus = att.Status
	}
	return status, nil
}

func fallbackCommitInfo() (gitutil.CommitInfo, error) {
	sha, err := currentDraftSHA()
	if err != nil || strings.TrimSpace(sha) == "" {
		return gitutil.CommitInfo{}, err
	}
	message, _ := gitutil.CommitMessage(sha)
	author, _ := gitutil.Git("log", "-1", "--format=%an", sha)
	committedISO, _ := gitutil.Git("log", "-1", "--format=%cI", sha)
	top, _ := gitutil.RepoTopLevel()

	committed := time.Now().UTC()
	if committedISO != "" {
		if parsed, err := time.Parse(time.RFC3339, committedISO); err == nil {
			committed = parsed
		}
	}

	changeID := gitutil.ExtractChangeID(message)
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(sha)
	}

	repoName := ""
	if top != "" {
		repoName = filepath.Base(top)
	}

	return gitutil.CommitInfo{
		SHA:       strings.TrimSpace(sha),
		Author:    strings.TrimSpace(author),
		Message:   message,
		Committed: committed,
		RepoName:  repoName,
		ChangeID:  changeID,
		TopLevel:  top,
	}, nil
}
