package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/ci"
	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/notes"
)

func WriteAttestation(att client.Attestation) (client.Attestation, error) {
	return WriteAttestationTo(notes.RefAttestationsCheckpoint, att)
}

func WriteTraceAttestation(att client.Attestation) (client.Attestation, error) {
	return WriteAttestationTo(notes.RefAttestationsTrace, att)
}

func WriteAttestationTo(ref string, att client.Attestation) (client.Attestation, error) {
	if att.AttestationID == "" {
		att.AttestationID = newID()
	}
	if att.CreatedAt.IsZero() {
		att.CreatedAt = time.Now().UTC()
	}
	stored := att
	if config.CISyncOutput() {
		stored = scrubAttestationSignals(stored)
	} else {
		stored = stripAttestationSignals(stored)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := notes.AddJSON(ref, stored.CommitSHA, stored); err != nil {
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
	return GetAttestationFrom(notes.RefAttestationsCheckpoint, commitSHA)
}

func GetTraceAttestation(commitSHA string) (*client.Attestation, error) {
	return GetAttestationFrom(notes.RefAttestationsTrace, commitSHA)
}

func GetAttestationFrom(ref, commitSHA string) (*client.Attestation, error) {
	var att client.Attestation
	found, err := notes.ReadJSON(ref, commitSHA, &att)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &att, nil
}

func GetAttestationWithInheritance(commitSHA string) (*client.Attestation, *client.Attestation, error) {
	att, err := GetAttestation(commitSHA)
	if err != nil || att == nil {
		return att, nil, err
	}
	inheritFrom := strings.TrimSpace(att.AttestationInheritFrom)
	if inheritFrom == "" {
		return att, nil, nil
	}
	inherited, err := GetAttestation(inheritFrom)
	if err != nil {
		return att, nil, err
	}
	return att, inherited, nil
}

func WriteAttestationInheritance(commitSHA, inheritFrom string) error {
	commitSHA = strings.TrimSpace(commitSHA)
	inheritFrom = strings.TrimSpace(inheritFrom)
	if commitSHA == "" || inheritFrom == "" {
		return fmt.Errorf("commit and inherit-from required")
	}
	existing, err := GetAttestation(commitSHA)
	if err != nil {
		return err
	}
	if existing != nil {
		if strings.TrimSpace(existing.Status) != "" || strings.TrimSpace(existing.AttestationInheritFrom) != "" {
			return nil
		}
	}
	changeID := ""
	if msg, err := gitutil.CommitMessage(commitSHA); err == nil {
		changeID = gitutil.ExtractChangeID(msg)
	}
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(commitSHA)
	}
	att := client.Attestation{
		AttestationID:          newID(),
		CommitSHA:              commitSHA,
		ChangeID:               changeID,
		AttestationInheritFrom: inheritFrom,
		CreatedAt:              time.Now().UTC(),
	}
	return notes.AddJSON(notes.RefAttestationsCheckpoint, commitSHA, att)
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

func stripAttestationSignals(att client.Attestation) client.Attestation {
	if att.SignalsJSON == "" {
		return att
	}
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
	return att
}

func scrubAttestationSignals(att client.Attestation) client.Attestation {
	if att.SignalsJSON == "" {
		return att
	}
	var result ci.Result
	if err := json.Unmarshal([]byte(att.SignalsJSON), &result); err != nil {
		att.SignalsJSON = ""
		return att
	}
	for i := range result.Commands {
		result.Commands[i].OutputExcerpt = scrubSecrets(result.Commands[i].OutputExcerpt)
	}
	if encoded, err := json.Marshal(result); err == nil {
		att.SignalsJSON = string(encoded)
	} else {
		att.SignalsJSON = ""
	}
	return att
}

var attestationSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]+`),
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|pwd)\s*[:=]\s*\S+`),
}

func scrubSecrets(value string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	out := value
	for _, re := range attestationSecretPatterns {
		out = re.ReplaceAllString(out, "[redacted]")
	}
	return out
}
