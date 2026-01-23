package gitutil

import (
	"strings"
)

type Remote struct {
	Name string
	URL  string
}

func ListRemotes() ([]Remote, error) {
	out, err := git("remote")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var remotes []Remote
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		url, err := RemoteURL(name)
		if err != nil {
			continue
		}
		remotes = append(remotes, Remote{Name: name, URL: url})
	}
	return remotes, nil
}

func RemoteURL(name string) (string, error) {
	return git("remote", "get-url", name)
}

func RemoteExists(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	_, err := git("remote", "get-url", name)
	return err == nil
}
