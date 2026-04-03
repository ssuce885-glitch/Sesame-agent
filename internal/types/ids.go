package types

import (
	"crypto/rand"
	"encoding/hex"
)

func NewID(prefix string) string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])

	return prefix + "_" + hex.EncodeToString(buf[:])
}
