package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/gitutil"
)

type checkpointInfo struct {
	SHA       string
	ChangeID  string
	Message   string
	Author    string
	When      time.Time
	TraceHead string
}

func listCheckpoints() ([]checkpointInfo, error) {
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	prefix := keepRefPrefix(user, workspace)
	refs, err := listKeepRefs(prefix)
	if err != nil {
		return nil, err
	}

	entries := make([]checkpointInfo, 0, len(refs))
	for _, ref := range refs {
		info, err := checkpointFromRef(ref)
		if err != nil {
			continue
		}
		entries = append(entries, info)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].When.After(entries[j].When)
	})
	return entries, nil
}

func latestCheckpoint() (*checkpointInfo, error) {
	entries, err := listCheckpoints()
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return &entries[0], nil
}

func latestCheckpointForChange(changeID string) (*checkpointInfo, error) {
	entries, err := listCheckpoints()
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.ChangeID == changeID {
			return &entry, nil
		}
	}
	return nil, nil
}

func checkpointFromRef(ref keepRefInfo) (checkpointInfo, error) {
	if ref.CheckpointSHA == "" {
		return checkpointInfo{}, fmt.Errorf("checkpoint sha missing")
	}
	msg, err := gitutil.CommitMessage(ref.CheckpointSHA)
	if err != nil {
		return checkpointInfo{}, err
	}
	traceHead := gitutil.ExtractTraceHead(msg)
	author, _ := gitutil.Git("log", "-1", "--format=%an", ref.CheckpointSHA)
	whenStr, _ := gitutil.Git("log", "-1", "--format=%cI", ref.CheckpointSHA)
	when, _ := time.Parse(time.RFC3339, strings.TrimSpace(whenStr))
	changeID := gitutil.ExtractChangeID(msg)
	if changeID == "" {
		changeID = ref.ChangeID
	}
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(ref.CheckpointSHA)
	}
	return checkpointInfo{
		SHA:       ref.CheckpointSHA,
		ChangeID:  changeID,
		Message:   strings.TrimSpace(msg),
		Author:    strings.TrimSpace(author),
		When:      when,
		TraceHead: strings.TrimSpace(traceHead),
	}, nil
}
