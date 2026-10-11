package forgesec

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

var (
	keyMu sync.RWMutex
	forgeInternalKey = make([]byte, 32)
)

func init() {
	_, _ = rand.Read(forgeInternalKey)
}

// SignToken generates an unforgeable HMAC-SHA256 signature for a verified forge approval.
func SignToken(approver, author, state, commitSHA, source string) string {
	keyMu.RLock()
	defer keyMu.RUnlock()

	mac := hmac.New(sha256.New, forgeInternalKey)
	msg := fmt.Sprintf("%s|%s|%s|%s|%s", approver, author, state, commitSHA, source)
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyToken validates that an approval signature matches the internal forge key.
func VerifyToken(approver, author, state, commitSHA, source, signature string) bool {
	if strings.TrimSpace(signature) == "" {
		return false
	}
	expected := SignToken(approver, author, state, commitSHA, source)
	return hmac.Equal([]byte(signature), []byte(expected))
}
