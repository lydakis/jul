package metadata

import (
	"time"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/notes"
)

func WriteAttestation(att client.Attestation) (client.Attestation, error) {
	if att.AttestationID == "" {
		att.AttestationID = newID()
	}
	if att.CreatedAt.IsZero() {
		att.CreatedAt = time.Now().UTC()
	}
	if err := notes.AddJSON(notes.RefAttestationsCheckpoint, att.CommitSHA, att); err != nil {
		return client.Attestation{}, err
	}
	return att, nil
}

func GetAttestation(commitSHA string) (*client.Attestation, error) {
	var att client.Attestation
	found, err := notes.ReadJSON(notes.RefAttestationsCheckpoint, commitSHA, &att)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &att, nil
}
