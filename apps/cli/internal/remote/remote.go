package remote

import (
	"errors"
	"strings"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
)

var (
	ErrNoRemote       = errors.New("no remotes configured")
	ErrMultipleRemote = errors.New("multiple remotes configured")
	ErrRemoteMissing  = errors.New("configured remote not found")
)

type Selected struct {
	Name string
	URL  string
}

// Resolve returns the remote Jul should use, based on config + git remotes.
// If no remote is configured, ErrNoRemote is returned.
// If multiple remotes exist and no selection is configured, ErrMultipleRemote is returned.
func Resolve() (Selected, error) {
	configured := strings.TrimSpace(config.RemoteName())
	if configured != "" {
		if !gitutil.RemoteExists(configured) {
			return Selected{}, ErrRemoteMissing
		}
		url, err := gitutil.RemoteURL(configured)
		if err != nil {
			return Selected{}, err
		}
		if override := strings.TrimSpace(config.RemoteURL()); override != "" {
			url = override
		}
		return Selected{Name: configured, URL: url}, nil
	}

	remotes, err := gitutil.ListRemotes()
	if err != nil {
		return Selected{}, err
	}
	if len(remotes) == 0 {
		return Selected{}, ErrNoRemote
	}
	for _, rem := range remotes {
		if rem.Name == "origin" {
			if override := strings.TrimSpace(config.RemoteURL()); override != "" {
				rem.URL = override
			}
			return Selected{Name: rem.Name, URL: rem.URL}, nil
		}
	}
	if len(remotes) == 1 {
		rem := remotes[0]
		if override := strings.TrimSpace(config.RemoteURL()); override != "" {
			rem.URL = override
		}
		return Selected{Name: rem.Name, URL: rem.URL}, nil
	}
	return Selected{}, ErrMultipleRemote
}
