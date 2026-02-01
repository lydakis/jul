package cli

import (
	"flag"
	"io"
	"os"
	"strings"
)

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

func stripJSONFlag(args []string) (bool, []string) {
	if len(args) == 0 {
		return false, args
	}
	filtered := make([]string, 0, len(args))
	jsonOut := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOut = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return jsonOut, filtered
}

func stripWatchFlag(args []string) (bool, bool, []string) {
	if len(args) == 0 {
		return false, false, args
	}
	filtered := make([]string, 0, len(args))
	watch := false
	set := false
	for _, arg := range args {
		if arg == "--watch" {
			watch = true
			set = true
			continue
		}
		if strings.HasPrefix(arg, "--watch=") {
			value := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--watch=")))
			switch value {
			case "", "1", "true", "yes", "on":
				watch = true
			case "0", "false", "no", "off":
				watch = false
			default:
				watch = true
			}
			set = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return watch, set, filtered
}

func ensureJSONFlag(args []string) []string {
	if hasJSONFlag(args) {
		return args
	}
	for i, arg := range args {
		if arg == "--" {
			out := make([]string, 0, len(args)+1)
			out = append(out, args[:i]...)
			out = append(out, "--json")
			out = append(out, args[i:]...)
			return out
		}
	}
	return append([]string{"--json"}, args...)
}

func newFlagSet(name string) (*flag.FlagSet, *bool) {
	return newFlagSetWithOutput(name, os.Stdout)
}

func newFlagSetWithOutput(name string, out io.Writer) (*flag.FlagSet, *bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	jsonOut := fs.Bool("json", false, "Output JSON")
	return fs, jsonOut
}
