package output

import (
	"fmt"
	"io"

	"github.com/lydakis/jul/cli/internal/syncer"
)

func RenderCheckpoint(w io.Writer, res syncer.CheckpointResult) {
	fmt.Fprintf(w, "checkpoint %s (%s)\n", res.CheckpointSHA, res.ChangeID)
}
