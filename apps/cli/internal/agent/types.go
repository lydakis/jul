package agent

import "encoding/json"

type ReviewRequest struct {
	Version       int           `json:"version"`
	Action        string        `json:"action"`
	WorkspacePath string        `json:"workspace_path"`
	Context       ReviewContext `json:"context"`
}

type ReviewContext struct {
	Checkpoint string          `json:"checkpoint,omitempty"`
	ChangeID   string          `json:"change_id,omitempty"`
	Diff       string          `json:"diff,omitempty"`
	Files      []ReviewFile    `json:"files,omitempty"`
	Conflicts  []string        `json:"conflicts,omitempty"`
	CIResults  json.RawMessage `json:"ci_results,omitempty"`
}

type ReviewFile struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
}

type ReviewResponse struct {
	Version     int                `json:"version"`
	Status      string             `json:"status"`
	Suggestions []ReviewSuggestion `json:"suggestions,omitempty"`
}

type ReviewSuggestion struct {
	ID           string   `json:"id,omitempty"`
	Commit       string   `json:"commit"`
	Reason       string   `json:"reason,omitempty"`
	Description  string   `json:"description,omitempty"`
	Confidence   float64  `json:"confidence,omitempty"`
	FilesChanged []string `json:"files_changed,omitempty"`
}
