package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/output"
)

type localManifest struct {
	Name      string    `json:"name"`
	SavedAt   time.Time `json:"saved_at"`
	Modified  int       `json:"modified"`
	Untracked int       `json:"untracked"`
}

type localState struct {
	Name      string
	SavedAt   time.Time
	Modified  int
	Untracked int
}

type localSaveOutput struct {
	Status string        `json:"status"`
	State  localManifest `json:"state"`
}

type localRestoreOutput struct {
	Status string `json:"status"`
	Name   string `json:"name"`
}

type localListOutput struct {
	States []localManifest `json:"states"`
}

type localDeleteOutput struct {
	Status string `json:"status"`
	Name   string `json:"name"`
}

func newLocalCommand() Command {
	return Command{
		Name:    "local",
		Summary: "Manage local workspace snapshots",
		Run: func(args []string) int {
			jsonOut, args := stripJSONFlag(args)
			if len(args) == 0 {
				if jsonOut {
					_ = output.EncodeError(os.Stdout, "local_missing_subcommand", "missing local subcommand", nil)
					return 1
				}
				printLocalUsage()
				return 1
			}
			sub := args[0]
			subArgs := args[1:]
			if jsonOut {
				subArgs = ensureJSONFlag(subArgs)
			}
			switch sub {
			case "save":
				return runLocalSave(subArgs)
			case "restore":
				return runLocalRestore(subArgs)
			case "list":
				return runLocalList(subArgs)
			case "delete":
				return runLocalDelete(subArgs)
			default:
				if jsonOut {
					_ = output.EncodeError(os.Stdout, "local_unknown_subcommand", fmt.Sprintf("unknown subcommand %q", sub), nil)
					return 1
				}
				printLocalUsage()
				return 1
			}
		},
	}
}

func runLocalSave(args []string) int {
	fs, jsonOut := newFlagSet("local save")
	_ = fs.Parse(args)
	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "local_missing_name", "name required", nil)
		} else {
			fmt.Fprintln(os.Stderr, "name required")
		}
		return 1
	}
	state, err := localSave(name)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "local_save_failed", fmt.Sprintf("save failed: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "save failed: %v\n", err)
		}
		return 1
	}
	out := localSaveOutput{Status: "ok", State: manifestFromState(state)}
	if *jsonOut {
		return writeJSON(out)
	}
	renderLocalSave(out)
	return 0
}

func runLocalRestore(args []string) int {
	fs, jsonOut := newFlagSet("local restore")
	_ = fs.Parse(args)
	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "local_missing_name", "name required", nil)
		} else {
			fmt.Fprintln(os.Stderr, "name required")
		}
		return 1
	}
	if err := localRestore(name); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "local_restore_failed", fmt.Sprintf("restore failed: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "restore failed: %v\n", err)
		}
		return 1
	}
	out := localRestoreOutput{Status: "ok", Name: name}
	if *jsonOut {
		return writeJSON(out)
	}
	renderLocalRestore(out)
	return 0
}

func runLocalList(args []string) int {
	fs, jsonOut := newFlagSet("local list")
	_ = fs.Parse(args)
	states, err := localList()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "local_list_failed", fmt.Sprintf("list failed: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "list failed: %v\n", err)
		}
		return 1
	}
	out := localListOutput{States: make([]localManifest, 0, len(states))}
	for _, state := range states {
		out.States = append(out.States, manifestFromState(state))
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderLocalList(out)
	return 0
}

func runLocalDelete(args []string) int {
	fs, jsonOut := newFlagSet("local delete")
	_ = fs.Parse(args)
	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "local_missing_name", "name required", nil)
		} else {
			fmt.Fprintln(os.Stderr, "name required")
		}
		return 1
	}
	if err := localDelete(name); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "local_delete_failed", fmt.Sprintf("delete failed: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "delete failed: %v\n", err)
		}
		return 1
	}
	out := localDeleteOutput{Status: "ok", Name: name}
	if *jsonOut {
		return writeJSON(out)
	}
	renderLocalDelete(out)
	return 0
}

func manifestFromState(state localState) localManifest {
	return localManifest{
		Name:      state.Name,
		SavedAt:   state.SavedAt,
		Modified:  state.Modified,
		Untracked: state.Untracked,
	}
}

func renderLocalSave(out localSaveOutput) {
	fmt.Fprintf(os.Stdout, "Saved local state '%s'\n", out.State.Name)
	fmt.Fprintf(os.Stdout, "  %d modified files\n", out.State.Modified)
	fmt.Fprintf(os.Stdout, "  %d untracked files\n", out.State.Untracked)
}

func renderLocalRestore(out localRestoreOutput) {
	fmt.Fprintf(os.Stdout, "Restored local state '%s'\n", out.Name)
}

func renderLocalList(out localListOutput) {
	if len(out.States) == 0 {
		fmt.Fprintln(os.Stdout, "No local states.")
		return
	}
	for _, state := range out.States {
		total := state.Modified + state.Untracked
		fmt.Fprintf(os.Stdout, "  %s (%d files, %s)\n", state.Name, total, humanTime(state.SavedAt))
	}
}

func renderLocalDelete(out localDeleteOutput) {
	fmt.Fprintf(os.Stdout, "Deleted local state '%s'\n", out.Name)
}

func localSave(name string) (localState, error) {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return localState{}, err
	}
	stateDir := filepath.Join(repoRoot, ".jul", "local", name)
	if _, err := os.Stat(stateDir); err == nil {
		return localState{}, fmt.Errorf("local state already exists: %s", name)
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "worktree"), 0o755); err != nil {
		return localState{}, err
	}
	if err := copyWorktree(repoRoot, filepath.Join(stateDir, "worktree")); err != nil {
		return localState{}, err
	}
	indexPath, err := gitutil.GitPath(repoRoot, "index")
	if err != nil {
		return localState{}, err
	}
	if _, err := os.Stat(indexPath); err != nil {
		if os.IsNotExist(err) {
			if _, headErr := gitutil.Git("rev-parse", "--verify", "HEAD"); headErr == nil {
				_, _ = gitutil.Git("read-tree", "HEAD")
			} else {
				_, _ = gitutil.Git("read-tree", "--empty")
			}
		} else {
			return localState{}, err
		}
	}
	if err := copyFile(indexPath, filepath.Join(stateDir, "index")); err != nil {
		return localState{}, err
	}
	modified, untracked, err := localStatusCounts()
	if err != nil {
		return localState{}, err
	}
	manifest := localManifest{
		Name:      name,
		SavedAt:   time.Now().UTC(),
		Modified:  modified,
		Untracked: untracked,
	}
	if err := writeManifest(stateDir, manifest); err != nil {
		return localState{}, err
	}
	return localState{
		Name:      manifest.Name,
		SavedAt:   manifest.SavedAt,
		Modified:  manifest.Modified,
		Untracked: manifest.Untracked,
	}, nil
}

func localRestore(name string) error {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return err
	}
	stateDir := filepath.Join(repoRoot, ".jul", "local", name)
	if _, err := os.Stat(stateDir); err != nil {
		return fmt.Errorf("local state not found: %s", name)
	}
	if _, err := gitutil.Git("reset", "--hard"); err != nil {
		return err
	}
	if _, err := gitutil.Git("clean", "-fd", "--exclude=.jul"); err != nil {
		return err
	}
	worktreeDir := filepath.Join(stateDir, "worktree")
	if err := copyWorktree(worktreeDir, repoRoot); err != nil {
		return err
	}
	indexPath, err := gitutil.GitPath(repoRoot, "index")
	if err != nil {
		return err
	}
	if err := copyFile(filepath.Join(stateDir, "index"), indexPath); err != nil {
		return err
	}
	return nil
}

func localList() ([]localState, error) {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(repoRoot, ".jul", "local")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []localState{}, nil
		}
		return nil, err
	}
	states := make([]localState, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, ok, err := readManifest(filepath.Join(root, entry.Name()))
		if err != nil || !ok {
			continue
		}
		states = append(states, localState{
			Name:      manifest.Name,
			SavedAt:   manifest.SavedAt,
			Modified:  manifest.Modified,
			Untracked: manifest.Untracked,
		})
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].SavedAt.After(states[j].SavedAt)
	})
	return states, nil
}

func localDelete(name string) error {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return err
	}
	stateDir := filepath.Join(repoRoot, ".jul", "local", name)
	if _, err := os.Stat(stateDir); err != nil {
		return fmt.Errorf("local state not found: %s", name)
	}
	return os.RemoveAll(stateDir)
}

func localStatusCounts() (int, int, error) {
	out, err := gitutil.Git("status", "--porcelain")
	if err != nil {
		return 0, 0, err
	}
	modified := 0
	untracked := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "??") {
			untracked++
			continue
		}
		modified++
	}
	return modified, untracked, nil
}

func writeManifest(stateDir string, manifest localManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, "manifest.json"), data, 0o644)
}

func readManifest(stateDir string) (localManifest, bool, error) {
	data, err := os.ReadFile(filepath.Join(stateDir, "manifest.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return localManifest{}, false, nil
		}
		return localManifest{}, false, err
	}
	var manifest localManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return localManifest{}, false, err
	}
	return manifest, true, nil
}

func copyWorktree(srcRoot, destRoot string) error {
	return filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		parts := strings.Split(rel, string(os.PathSeparator))
		if len(parts) > 0 {
			if parts[0] == ".git" || parts[0] == ".jul" {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		target := filepath.Join(destRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFileWithMode(path, target, info.Mode())
	})
}

func copyFileWithMode(src, dest string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyFile(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return copyFileWithMode(src, dest, info.Mode())
}

func humanTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Format("2006-01-02 15:04")
}

func printLocalUsage() {
	fmt.Fprintln(os.Stdout, "Usage: jul local [save|restore|list|delete]")
}
