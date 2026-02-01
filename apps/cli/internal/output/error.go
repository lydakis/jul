package output

import "io"

type ErrorOutput struct {
	Code        string       `json:"code"`
	Message     string       `json:"message"`
	NextActions []NextAction `json:"next_actions,omitempty"`
}

func EncodeError(w io.Writer, code, message string, next []NextAction) error {
	payload := ErrorOutput{
		Code:        code,
		Message:     message,
		NextActions: next,
	}
	return EncodeJSON(w, payload)
}
