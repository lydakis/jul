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

func listCheckpoints(limit int) ([]checkpointInfo, error) {
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	prefix := keepRefPrefix(user, workspace)
	var refs []keepRefInfo
	var err error
	if limit > 0 {
		refs, err = listKeepRefsLimited(prefix, limit)
	} else {
		refs, err = listKeepRefs(prefix)
	}
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
	entries, err := listCheckpoints(1)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return &entries[0], nil
}

func latestCheckpointForChange(changeID string) (*checkpointInfo, error) {
	entries, err := listCheckpoints(0)
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
	out, err := gitutil.Git("log", "-1", "--format=%an%x00%cI%x00%B", ref.CheckpointSHA)
	if err != nil {
		return checkpointInfo{}, err
	}
	parts := strings.SplitN(out, "\x00", 3)
	if len(parts) < 3 {
		return checkpointInfo{}, fmt.Errorf("unexpected checkpoint metadata")
	}
	author := strings.TrimSpace(parts[0])
	whenStr := strings.TrimSpace(parts[1])
	msg := parts[2]
	traceHead := gitutil.ExtractTraceHead(msg)
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
