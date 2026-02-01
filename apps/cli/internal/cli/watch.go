package cli

import (
	"io"
	"os"
	"strings"
)

func watchEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("JUL_WATCH")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func watchStream(jsonOut bool, out io.Writer, errOut io.Writer) io.Writer {
	if !watchEnabled() {
		return nil
	}
	if jsonOut {
		return errOut
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("JUL_WATCH_STREAM")), "stdout") {
		return out
	}
	return errOut
}
