package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// promptCacheKeyPrefix names where the key came from, so a key seen in a
// provider dashboard or log is recognizably TARS's.
const promptCacheKeyPrefix = "tars-"

// promptCacheKey turns ChatOptions.CacheKey into the prompt_cache_key value
// OpenAI-family requests carry, or "" when the caller set none.
//
// The caller's identifier (a chat session id) is hashed rather than sent:
// the provider only needs the same key for the same session, not the id
// itself. 32 hex characters keep the key well under the API's 64-character
// limit while leaving collisions out of reach for one person's sessions.
func promptCacheKey(cacheKey string) string {
	trimmed := strings.TrimSpace(cacheKey)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return promptCacheKeyPrefix + hex.EncodeToString(sum[:16])
}
