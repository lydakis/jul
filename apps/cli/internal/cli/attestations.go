package cli

import (
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/metadata"
)

type attestationView struct {
	Status        string
	Stale         bool
	InheritedFrom string
	Attestation   *client.Attestation
}

func resolveAttestationView(commitSHA string) (attestationView, error) {
	if strings.TrimSpace(commitSHA) == "" {
		return attestationView{}, nil
	}
	att, inherited, err := metadata.GetAttestationWithInheritance(commitSHA)
	if err != nil || att == nil {
		return attestationView{}, err
	}
	status := strings.TrimSpace(att.Status)
	if status != "" {
		return attestationView{Status: status, Attestation: att}, nil
	}
	inheritFrom := strings.TrimSpace(att.AttestationInheritFrom)
	if inheritFrom != "" && inherited != nil {
		inhStatus := strings.TrimSpace(inherited.Status)
		if inhStatus != "" {
			return attestationView{
				Status:        inhStatus,
				Stale:         true,
				InheritedFrom: inheritFrom,
				Attestation:   inherited,
			}, nil
		}
	}
	return attestationView{
		Attestation:   att,
		InheritedFrom: inheritFrom,
	}, nil
}
