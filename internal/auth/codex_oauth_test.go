package auth

import "testing"

func withCodexRefreshTokenStoreForTests(t *testing.T, store codexRefreshTokenStore) {
	t.Helper()
	prev := currentCodexRefreshTokenStore
	currentCodexRefreshTokenStore = func() codexRefreshTokenStore {
		return store
	}
	t.Cleanup(func() {
		currentCodexRefreshTokenStore = prev
	})
}
