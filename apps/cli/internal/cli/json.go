package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/lydakis/jul/cli/internal/output"
)

func writeJSON(payload any) int {
	return writeJSONTo(os.Stdout, payload)
}

func writeJSONTo(w io.Writer, payload any) int {
	if err := output.EncodeJSON(w, payload); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
		return 1
	}
	return 0
}
