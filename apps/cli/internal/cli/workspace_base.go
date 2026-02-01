package cli

import (
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/workspace"
)

func detectBaseRef(repoRoot string) string {
	if strings.TrimSpace(repoRoot) == "" {
		return ""
	}
	if ref, err := gitutil.Git("-C", repoRoot, "symbolic-ref", "-q", "HEAD"); err == nil {
		ref = strings.TrimSpace(ref)
		if ref != "" && !strings.HasPrefix(ref, "refs/heads/jul/") {
			return ref
		}
	}
	if refExists(repoRoot, "refs/heads/main") {
		return "refs/heads/main"
	}
	if refExists(repoRoot, "refs/heads/master") {
		return "refs/heads/master"
	}
	return ""
}

func refExists(repoRoot, ref string) bool {
	if strings.TrimSpace(ref) == "" {
		return false
	}
	_, err := gitutil.Git("-C", repoRoot, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

func ensureWorkspaceConfig(repoRoot, workspaceName, baseRef, baseSHA string) error {
	trackRef := trackRefForBase(baseRef)
	trackTip := ""
	if trackRef != "" {
		trackTip = strings.TrimSpace(baseSHA)
	}
	cfg := workspace.Config{
		BaseRef:  strings.TrimSpace(baseRef),
		BaseSHA:  strings.TrimSpace(baseSHA),
		TrackRef: trackRef,
		TrackTip: trackTip,
	}
	return workspace.WriteConfig(repoRoot, workspaceName, cfg)
}

func trackRefForBase(baseRef string) string {
	trimmed := strings.TrimSpace(baseRef)
	if strings.HasPrefix(trimmed, "refs/heads/") {
		return trimmed
	}
	return ""
}

func updateWorkspaceTracking(repoRoot, workspaceName, trackRef, trackTip string) error {
	cfg, _, err := workspace.ReadConfig(repoRoot, workspaceName)
	if err != nil {
		return err
	}
	if strings.TrimSpace(trackRef) != "" {
		cfg.TrackRef = strings.TrimSpace(trackRef)
	}
	if strings.TrimSpace(trackTip) != "" {
		cfg.TrackTip = strings.TrimSpace(trackTip)
	}
	return workspace.WriteConfig(repoRoot, workspaceName, cfg)
}

func refreshWorkspaceTrackTip(repoRoot, workspaceName string) {
	cfg, ok, err := workspace.ReadConfig(repoRoot, workspaceName)
	if err != nil || !ok {
		return
	}
	if cfg.TrackRef == "" {
		cfg.TrackRef = trackRefForBase(cfg.BaseRef)
	}
	if cfg.TrackRef == "" {
		return
	}
	tip, err := gitutil.Git("-C", repoRoot, "rev-parse", cfg.TrackRef)
	if err != nil {
		return
	}
	cfg.TrackTip = strings.TrimSpace(tip)
	_ = workspace.WriteConfig(repoRoot, workspaceName, cfg)
}
