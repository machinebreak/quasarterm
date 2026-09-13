// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"bytes"
	"encoding/json"
	"os"
	"time"
)

// credentialExpiryMillis extracts the best-known expiry timestamp (in unix
// milliseconds) from a stored credential blob. Returns 0 when the expiry
// cannot be determined for that provider.
func credentialExpiryMillis(credentials []byte) int64 {
	var root map[string]any
	if err := json.Unmarshal(credentials, &root); err != nil {
		return 0
	}
	// Codex: tokens.access_token / tokens.id_token (JWT exp, seconds)
	if tokensMap, ok := root["tokens"].(map[string]any); ok {
		for _, key := range []string{"access_token", "id_token"} {
			claims := decodeJWTPayload(jsonString(tokensMap[key]))
			if claims == nil {
				continue
			}
			if exp, ok := jsonFloat(claims["exp"]); ok {
				return int64(exp) * 1000
			}
		}
	}
	// Claude: claudeAiOauth.expiresAt (unix milliseconds)
	if oauthMap, ok := root["claudeAiOauth"].(map[string]any); ok {
		if exp, ok := jsonFloat(oauthMap["expiresAt"]); ok {
			return int64(exp)
		}
	}
	// Antigravity: token.expiry (RFC3339)
	if tokenMap, ok := root["token"].(map[string]any); ok {
		if parsed, err := time.Parse(time.RFC3339, jsonString(tokenMap["expiry"])); err == nil {
			return parsed.UnixMilli()
		}
	}
	return 0
}

// preferNewerCredentials decides whether the live credentials should replace
// the stored ones. The live file is what the CLI itself wrote most recently,
// so it wins unless it is provably older (both expiries known and live older).
func preferNewerCredentials(live []byte, stored []byte) bool {
	if bytes.Equal(live, stored) {
		return false
	}
	liveExpiry := credentialExpiryMillis(live)
	storedExpiry := credentialExpiryMillis(stored)
	if liveExpiry > 0 && storedExpiry > 0 && liveExpiry < storedExpiry {
		return false
	}
	return true
}

// syncActiveCredentialsToStore captures the provider's current on-disk
// credentials into the stored account they belong to. CLIs refresh their own
// tokens while running; without this sync the stored copy goes stale and the
// next switch (or instance/wake launch) would overwrite the live file with an
// old token, reverting the account to a revoked or expired session.
func (m *Manager) syncActiveCredentialsToStore(provider Provider) {
	path, err := provider.CurrentCredentialsPath()
	if err != nil {
		return
	}
	live, err := os.ReadFile(path)
	if err != nil || len(live) == 0 {
		return
	}
	key, err := provider.RemoteKey(live)
	if err != nil || key == "" {
		return
	}
	idx, err := loadIndex()
	if err != nil {
		return
	}
	providerID := provider.Info().ID
	for i := range idx.Accounts {
		account := idx.Accounts[i]
		if account.Provider != providerID || account.RemoteID != key {
			continue
		}
		if stored, storedErr := loadCredentials(providerID, account.ID); storedErr == nil {
			if !preferNewerCredentials(live, stored) {
				return
			}
		}
		saveCredentials(providerID, account.ID, live)
		return
	}
}
