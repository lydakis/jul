package cli

import (
	"fmt"
	"strings"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
)

func workspaceParts() (string, string) {
	id := strings.TrimSpace(config.WorkspaceID())
	parts := strings.SplitN(id, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	user := config.UserName()
	if user == "" {
		user = "user"
	}
	return user, id
}

func workspaceRef(user, workspace string) string {
	return fmt.Sprintf("refs/jul/workspaces/%s/%s", user, workspace)
}

func syncRef(user, workspace string) (string, error) {
	deviceID, err := config.DeviceID()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("refs/jul/sync/%s/%s/%s", user, deviceID, workspace), nil
}

func keepRefPrefix(user, workspace string) string {
	return fmt.Sprintf("refs/jul/keep/%s/%s/", user, workspace)
}

type keepRefInfo struct {
	Ref           string
	SHA           string
	ChangeID      string
	CheckpointSHA string
}

func listKeepRefs(prefix string) ([]keepRefInfo, error) {
	if strings.TrimSpace(prefix) == "" {
		return nil, fmt.Errorf("keep ref prefix required")
	}
	out, err := gitutil.Git("show-ref")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var refs []keepRefInfo
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sha := fields[0]
		ref := fields[1]
		if !strings.HasPrefix(ref, prefix) {
			continue
		}
		rest := strings.TrimPrefix(ref, prefix)
		parts := strings.Split(rest, "/")
		if len(parts) < 2 {
			continue
		}
		changeID := parts[0]
		checkpoint := parts[1]
		refs = append(refs, keepRefInfo{
			Ref:           ref,
			SHA:           sha,
			ChangeID:      changeID,
			CheckpointSHA: checkpoint,
		})
	}
	return refs, nil
}
