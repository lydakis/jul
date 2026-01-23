package output

import (
	"fmt"
	"io"
	"strings"
)

type DiffResult struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Diff string `json:"diff,omitempty"`
}

func RenderDiff(w io.Writer, res DiffResult) {
	out := strings.TrimRight(res.Diff, "\n")
	fmt.Fprintln(w, out)
}
