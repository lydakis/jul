package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
)

func newDoctorCommand() Command {
	return Command{
		Name:    "doctor",
		Summary: "Probe remote compatibility for Jul sync",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			_ = fs.Parse(args)

			if err := runDoctor(); err != nil {
				fmt.Fprintf(os.Stderr, "doctor failed: %v\n", err)
				return 1
			}
			return 0
		},
	}
}

func runDoctor() error {
	remote, err := remotesel.Resolve()
	if err != nil {
		switch err {
		case remotesel.ErrNoRemote, remotesel.ErrRemoteMissing:
			fmt.Fprintln(os.Stdout, "No sync remote configured; draft and checkpoint sync disabled.")
			if err := config.SetRepoConfigValue("remote", "checkpoint_sync", "disabled"); err != nil {
				return err
			}
			if err := config.SetRepoConfigValue("remote", "draft_sync", "disabled"); err != nil {
				return err
			}
			return nil
		case remotesel.ErrMultipleRemote:
			return fmt.Errorf("multiple remotes found; run 'jul remote set <name>'")
		default:
			return err
		}
	}

	headSHA, err := gitutil.Git("rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(headSHA) == "" {
		fmt.Fprintln(os.Stdout, "No commits found; sync probes skipped.")
		if err := config.SetRepoConfigValue("remote", "checkpoint_sync", "disabled"); err != nil {
			return err
		}
		if err := config.SetRepoConfigValue("remote", "draft_sync", "disabled"); err != nil {
			return err
		}
		return nil
	}
	headSHA = strings.TrimSpace(headSHA)
	deviceID, err := config.DeviceID()
	if err != nil {
		return err
	}
	ref := fmt.Sprintf("refs/jul/doctor/%s", strings.TrimSpace(deviceID))
	noteRef := "refs/notes/jul/doctor"

	checkpointOK, draftOK, err := probeSyncCapabilities(remote.Name, headSHA, ref, noteRef)
	if err != nil {
		return err
	}

	checkpointState := "disabled"
	if checkpointOK {
		checkpointState = "enabled"
	}
	draftState := "disabled"
	if draftOK {
		draftState = "enabled"
	}

	if err := config.SetRepoConfigValue("remote", "checkpoint_sync", checkpointState); err != nil {
		return err
	}
	if err := config.SetRepoConfigValue("remote", "draft_sync", draftState); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "checkpoint_sync: %s\n", checkpointState)
	fmt.Fprintf(os.Stdout, "draft_sync: %s\n", draftState)
	return nil
}

func probeSyncCapabilities(remoteName, headSHA, ref, noteRef string) (bool, bool, error) {
	checkpointOK := false
	draftOK := false

	if err := pushRef(remoteName, headSHA, ref, false); err != nil {
		return false, false, err
	}
	if _, err := gitutil.Git("notes", "--ref", noteRef, "add", "-f", "-m", "jul doctor", headSHA); err != nil {
		_, _ = gitutil.Git("push", remoteName, ":"+ref)
		return false, false, err
	}
	if _, err := gitutil.Git("push", remoteName, noteRef+":"+noteRef); err != nil {
		_, _ = gitutil.Git("notes", "--ref", noteRef, "remove", headSHA)
		_, _ = gitutil.Git("push", remoteName, ":"+ref)
		return false, false, err
	}
	checkpointOK = true

	parent, _ := gitutil.ParentOf(headSHA)
	if strings.TrimSpace(parent) != "" {
		// Attempt a non-fast-forward update (force-with-lease).
		spec := fmt.Sprintf("%s:%s", strings.TrimSpace(parent), ref)
		args := []string{"push", "--force-with-lease=" + ref + ":" + strings.TrimSpace(headSHA), remoteName, spec}
		if _, err := gitutil.Git(args...); err == nil {
			draftOK = true
		}
	}

	_, _ = gitutil.Git("notes", "--ref", noteRef, "remove", headSHA)
	_, _ = gitutil.Git("push", remoteName, ":"+ref)
	_, _ = gitutil.Git("push", remoteName, ":"+noteRef)
	return checkpointOK, draftOK, nil
}
