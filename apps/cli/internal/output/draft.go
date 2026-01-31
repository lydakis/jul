package output

import (
	"fmt"
	"io"
	"strings"
	"time"
)

type DraftInfo struct {
	Device       string    `json:"device"`
	Workspace    string    `json:"workspace"`
	DraftSHA     string    `json:"draft_sha"`
	BaseSHA      string    `json:"base_sha"`
	ChangeID     string    `json:"change_id,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
	FilesChanged int       `json:"files_changed,omitempty"`
}

type DraftList struct {
	Drafts []DraftInfo `json:"drafts"`
}

type DraftShow struct {
	Draft DraftInfo `json:"draft"`
}

func RenderDraftList(w io.Writer, drafts []DraftInfo, opts Options) {
	if len(drafts) == 0 {
		fmt.Fprintln(w, "No drafts found.")
		return
	}
	for _, draft := range drafts {
		device := strings.TrimSpace(draft.Device)
		if device == "" {
			device = "device"
		}
		workspace := strings.TrimSpace(draft.Workspace)
		if workspace == "" {
			workspace = "@"
		}
		base := shortID(draft.BaseSHA, 6)
		updated := formatAge(draft.UpdatedAt)
		if base != "" && updated != "" {
			fmt.Fprintf(w, "%-14s %s  base=%s  updated=%s\n", device, workspace, base, updated)
		} else if base != "" {
			fmt.Fprintf(w, "%-14s %s  base=%s\n", device, workspace, base)
		} else {
			fmt.Fprintf(w, "%-14s %s\n", device, workspace)
		}
	}
}

func RenderDraftShow(w io.Writer, draft DraftInfo, opts Options) {
	writeKV(w, "Device", draft.Device, 10)
	writeKV(w, "Workspace", draft.Workspace, 10)
	writeKV(w, "Draft", shortID(draft.DraftSHA, 10), 10)
	writeKV(w, "Base", shortID(draft.BaseSHA, 10), 10)
	writeKV(w, "Change", shortID(draft.ChangeID, 10), 10)
	if draft.FilesChanged > 0 {
		writeKV(w, "Files", fmt.Sprintf("%d", draft.FilesChanged), 10)
	}
	if updated := formatAge(draft.UpdatedAt); updated != "" {
		writeKV(w, "Updated", updated, 10)
	}
}

func formatAge(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	age := time.Since(ts)
	if age < time.Minute {
		return "just now"
	}
	if age < time.Hour {
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	}
	if age < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(age.Hours()))
	}
	days := int(age.Hours() / 24)
	return fmt.Sprintf("%dd ago", days)
}
