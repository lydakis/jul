package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lydakis/jul/cli/internal/ci"
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
	stored := att
	for attempt := 0; attempt < 3; attempt++ {
		if err := notes.AddJSON(notes.RefAttestationsCheckpoint, stored.CommitSHA, stored); err != nil {
			if errors.Is(err, notes.ErrNoteTooLarge) {
				stored = shrinkAttestationSignals(stored, attempt)
				continue
			}
			return client.Attestation{}, err
		}
		return stored, nil
	}
	return client.Attestation{}, fmt.Errorf("attestation exceeds note size limit")
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

func shrinkAttestationSignals(att client.Attestation, attempt int) client.Attestation {
	if att.SignalsJSON == "" {
		return att
	}
	switch attempt {
	case 0:
		var result ci.Result
		if err := json.Unmarshal([]byte(att.SignalsJSON), &result); err != nil {
			att.SignalsJSON = ""
			return att
		}
		for i := range result.Commands {
			result.Commands[i].OutputExcerpt = ""
		}
		if encoded, err := json.Marshal(result); err == nil {
			att.SignalsJSON = string(encoded)
		} else {
			att.SignalsJSON = ""
		}
	case 1:
		att.SignalsJSON = ""
	default:
		att.SignalsJSON = ""
	}
	return att
}
