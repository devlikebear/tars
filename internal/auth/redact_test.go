package auth

import "fmt"

// redacted stands in for a credential value in a failure message. A test that
// goes wrong can be holding a real credential — one whose home was not
// isolated once read the user's own ~/.codex/auth.json on Windows — so its
// failure message must never print the value.
func redacted(secret string) string {
	return fmt.Sprintf("<redacted, %d bytes>", len(secret))
}

// describeCredential is %+v of a credential with its tokens redacted.
func describeCredential(cred CodexCredential) string {
	return fmt.Sprintf("{AccessToken:%s RefreshToken:%s AccountID:%s Source:%s SourcePath:%s}",
		redacted(cred.AccessToken), redacted(cred.RefreshToken), cred.AccountID, cred.Source, cred.SourcePath)
}
