package session

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func SpecialistSessionID(roleID, workspaceRoot string) string {
	roleID = strings.TrimSpace(roleID)
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	sum := sha256.Sum256([]byte(workspaceRoot))
	suffix := hex.EncodeToString(sum[:])[:8]
	return "specialist-" + roleID + "-" + suffix
}
