package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func startSyncSpinner(out io.Writer) func() {
	if out == nil || !isTerminalWriter(out) {
		return func() {}
	}

	started := time.Now()
	stop := make(chan struct{})
	done := make(chan struct{})
	frames := []rune{'|', '/', '-', '\\'}

	go func() {
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		defer close(done)

		index := 0
		for {
			select {
			case <-ticker.C:
				frame := frames[index%len(frames)]
				index++
				_, _ = fmt.Fprintf(out, "\rSyncing %c %s", frame, formatSyncElapsed(time.Since(started)))
			case <-stop:
				_, _ = fmt.Fprintf(out, "\r%s\r", strings.Repeat(" ", 48))
				return
			}
		}
	}()

	return func() {
		close(stop)
		<-done
	}
}

func startSyncWatchProgress(out io.Writer) func() {
	if out == nil {
		return func() {}
	}

	started := time.Now()
	_, _ = fmt.Fprintln(out, "sync: running...")

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		defer close(done)
		for {
			select {
			case <-ticker.C:
				_, _ = fmt.Fprintf(out, "sync: running (%s)\n", formatSyncElapsed(time.Since(started)))
			case <-stop:
				return
			}
		}
	}()

	return func() {
		close(stop)
		<-done
	}
}

func formatSyncElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%ds", int(d.Round(time.Second).Seconds()))
}

func isTerminalWriter(out io.Writer) bool {
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
