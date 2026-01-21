package gitutil

import (
	"crypto/rand"
	"encoding/hex"
)

func NewChangeID() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "I" + hex.EncodeToString(buf), nil
}
