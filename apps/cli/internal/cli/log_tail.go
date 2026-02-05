package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type logTail struct {
	Path   string
	Prefix string
}

func tailFile(ctx context.Context, path string, out io.Writer, prefix string) error {
	if strings.TrimSpace(path) == "" || out == nil {
		return nil
	}

	var file *os.File
	for file == nil {
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(150 * time.Millisecond):
					continue
				}
			}
			return err
		}
		file = f
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			writeTailLine(out, prefix, line)
		}
		if err == nil {
			continue
		}
		if err == io.EOF {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				continue
			}
		}
		return err
	}
}

func writeTailLine(out io.Writer, prefix, line string) {
	if out == nil || line == "" {
		return
	}
	if prefix == "" {
		fmt.Fprint(out, line)
		return
	}
	parts := strings.SplitAfter(line, "\n")
	for _, part := range parts {
		if part == "" {
			continue
		}
		fmt.Fprint(out, prefix)
		fmt.Fprint(out, part)
	}
}
