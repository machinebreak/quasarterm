// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func testJWT(expSeconds int64) string {
	payload, _ := json.Marshal(map[string]any{"exp": expSeconds})
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestPreferNewerCredentials(t *testing.T) {
	if preferNewerCredentials([]byte(`{"x":1}`), []byte(`{"x":1}`)) {
		t.Fatal("equal credentials should never be re-synced")
	}
	if !preferNewerCredentials([]byte(`{"x":2}`), []byte(`{"x":1}`)) {
		t.Fatal("live credentials with unknown expiry should win")
	}
	live := []byte(`{"tokens":{"access_token":"` + testJWT(2000000000) + `"}}`)
	stored := []byte(`{"tokens":{"access_token":"` + testJWT(1000000000) + `"}}`)
	if !preferNewerCredentials(live, stored) {
		t.Fatal("live token with a later expiry should replace the stored one")
	}
	if preferNewerCredentials(stored, live) {
		t.Fatal("older live credentials must not replace a newer stored token")
	}
	claudeLive := []byte(`{"claudeAiOauth":{"accessToken":"a","expiresAt":2000000000000}}`)
	claudeStored := []byte(`{"claudeAiOauth":{"accessToken":"b","expiresAt":1000000000000}}`)
	if !preferNewerCredentials(claudeLive, claudeStored) {
		t.Fatal("claude expiry comparison failed")
	}
	if preferNewerCredentials(claudeStored, claudeLive) {
		t.Fatal("older claude credentials must not replace a newer stored token")
	}
}

func TestCodexTokensNeedRefresh(t *testing.T) {
	future := testJWT(nowMillis()/1000 + 3600)
	past := testJWT(nowMillis()/1000 - 120)
	if codexTokensNeedRefresh(&codexAuthTokens{AccessToken: future, IDToken: future}) {
		t.Fatal("fresh tokens should not need a refresh")
	}
	if !codexTokensNeedRefresh(&codexAuthTokens{AccessToken: past, IDToken: future}) {
		t.Fatal("an expired access token should trigger a refresh")
	}
	if !codexTokensNeedRefresh(&codexAuthTokens{AccessToken: future, IDToken: past}) {
		t.Fatal("an expired id_token should trigger a refresh")
	}
}
