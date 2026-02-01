package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/output"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
)

type doctorOutput struct {
	Status         string `json:"status"`
	RemoteName     string `json:"remote_name,omitempty"`
	CheckpointSync string `json:"checkpoint_sync"`
	DraftSync      string `json:"draft_sync"`
	Message        string `json:"message,omitempty"`
}

func newDoctorCommand() Command {
	return Command{
		Name:    "doctor",
		Summary: "Probe remote compatibility for Jul sync",
		Run: func(args []string) int {
			fs, jsonOut := newFlagSet("doctor")
			_ = fs.Parse(args)

			out, err := runDoctor()
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "doctor_failed", err.Error(), nil)
				} else {
					fmt.Fprintf(os.Stderr, "doctor failed: %v\n", err)
				}
				return 1
			}
			if *jsonOut {
				return writeJSON(out)
			}
			renderDoctorOutput(out)
			return 0
		},
	}
}

func runDoctor() (doctorOutput, error) {
	out := doctorOutput{Status: "ok"}
	remote, err := remotesel.Resolve()
	if err != nil {
		switch err {
		case remotesel.ErrNoRemote, remotesel.ErrRemoteMissing:
			if err := config.SetRepoConfigValue("remote", "checkpoint_sync", "disabled"); err != nil {
				return out, err
			}
			if err := config.SetRepoConfigValue("remote", "draft_sync", "disabled"); err != nil {
				return out, err
			}
			out.CheckpointSync = "disabled"
			out.DraftSync = "disabled"
			out.Message = "No sync remote configured; draft and checkpoint sync disabled."
			return out, nil
		case remotesel.ErrMultipleRemote:
			return out, fmt.Errorf("multiple remotes found; run 'jul remote set <name>'")
		default:
			return out, err
		}
	}
	out.RemoteName = remote.Name

	headSHA, err := gitutil.Git("rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(headSHA) == "" {
		if err := config.SetRepoConfigValue("remote", "checkpoint_sync", "disabled"); err != nil {
			return out, err
		}
		if err := config.SetRepoConfigValue("remote", "draft_sync", "disabled"); err != nil {
			return out, err
		}
		out.CheckpointSync = "disabled"
		out.DraftSync = "disabled"
		out.Message = "No commits found; sync probes skipped."
		return out, nil
	}
	headSHA = strings.TrimSpace(headSHA)
	deviceID, err := config.DeviceID()
	if err != nil {
		return out, err
	}
	ref := fmt.Sprintf("refs/jul/doctor/%s", strings.TrimSpace(deviceID))
	noteRef := "refs/notes/jul/doctor"

	checkpointOK, draftOK, err := probeSyncCapabilities(remote.Name, headSHA, ref, noteRef)
	if err != nil {
		return out, err
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
		return out, err
	}
	if err := config.SetRepoConfigValue("remote", "draft_sync", draftState); err != nil {
		return out, err
	}

	out.CheckpointSync = checkpointState
	out.DraftSync = draftState
	return out, nil
}

func renderDoctorOutput(out doctorOutput) {
	if out.Message != "" {
		fmt.Fprintln(os.Stdout, out.Message)
	}
	if out.CheckpointSync != "" {
		fmt.Fprintf(os.Stdout, "checkpoint_sync: %s\n", out.CheckpointSync)
	}
	if out.DraftSync != "" {
		fmt.Fprintf(os.Stdout, "draft_sync: %s\n", out.DraftSync)
	}
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
