package output

import (
	"fmt"
	"io"
)

type ApplyResult struct {
	SuggestionID string        `json:"suggestion_id"`
	Applied      bool          `json:"applied"`
	FilesChanged []string      `json:"files_changed,omitempty"`
	Draft        ApplyDraft    `json:"draft,omitempty"`
	Checkpoint   string        `json:"checkpoint_sha,omitempty"`
	NextActions  []ApplyAction `json:"next_actions,omitempty"`
}

type ApplyDraft struct {
	ChangeID     string `json:"change_id,omitempty"`
	FilesChanged int    `json:"files_changed"`
}

type ApplyAction struct {
	Action  string `json:"action"`
	Command string `json:"command"`
}

func RenderApply(w io.Writer, res ApplyResult) {
	if res.Applied {
		fmt.Fprintln(w, "Applied to draft.")
	} else {
		fmt.Fprintln(w, "No changes applied.")
	}
	if res.Checkpoint != "" {
		fmt.Fprintf(w, "Checkpointed as %s\n", res.Checkpoint)
	}
}
