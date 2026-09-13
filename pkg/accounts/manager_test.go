// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "quasar-accounts-test-")
	if err != nil {
		panic(err)
	}
	prev := wavebase.DataHome_VarCache
	wavebase.DataHome_VarCache = dir
	code := m.Run()
	wavebase.DataHome_VarCache = prev
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestNormalizeTags(t *testing.T) {
	got := normalizeTags([]string{"  work ", "Work", "", "personal", "personal", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j"})
	want := []string{"work", "personal", "a", "b", "c", "d", "e", "f", "g", "h"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeTags mismatch: got %v want %v", got, want)
	}
	long := normalizeTags([]string{"0123456789012345678901234567890"})
	if len(long) != 1 || len(long[0]) != 24 {
		t.Fatalf("expected tag truncated to 24 chars, got %v", long)
	}
}

func TestNormalizeInstanceArgs(t *testing.T) {
	got := normalizeInstanceArgs([]string{" --verbose ", "", "  ", "--model=opus"})
	want := []string{"--verbose", "--model=opus"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeInstanceArgs mismatch: got %v want %v", got, want)
	}
}

func TestAccountTagsAndExportRoundTrip(t *testing.T) {
	manager := GetManager()
	seed := `{
  "version": 1,
  "exported": 1,
  "accounts": [
    {
      "id": "test-account-1",
      "provider": "claude",
      "email": "user@example.com",
      "credentials": {"claudeAiOauth": {"accessToken": "secret-token"}}
    }
  ]
}`
	imported, err := manager.ImportAccounts(seed)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected 1 imported account, got %d", imported)
	}

	if err := manager.SetTags("claude", "test-account-1", []string{"work", "work", " main "}); err != nil {
		t.Fatalf("set tags failed: %v", err)
	}
	list, err := manager.ListAccounts("claude")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	var found *Account
	for i := range list {
		if list[i].ID == "test-account-1" {
			found = &list[i]
		}
	}
	if found == nil {
		t.Fatal("account not found after import")
	}
	if !reflect.DeepEqual(found.Tags, []string{"work", "main"}) {
		t.Fatalf("unexpected tags: %v", found.Tags)
	}

	exported, err := manager.ExportAccount("claude", "test-account-1", true)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	var parsed accountsExportFile
	if err := json.Unmarshal([]byte(exported), &parsed); err != nil {
		t.Fatalf("exported json invalid: %v", err)
	}
	if len(parsed.Accounts) != 1 {
		t.Fatalf("expected 1 exported account, got %d", len(parsed.Accounts))
	}
	if len(parsed.Accounts[0].Credentials) == 0 {
		t.Fatal("expected exported credentials with includeCredentials=true")
	}
	if !reflect.DeepEqual(parsed.Accounts[0].Tags, []string{"work", "main"}) {
		t.Fatalf("exported tags mismatch: %v", parsed.Accounts[0].Tags)
	}

	redacted, err := manager.ExportAccount("claude", "test-account-1", false)
	if err != nil {
		t.Fatalf("redacted export failed: %v", err)
	}
	var redactedParsed accountsExportFile
	if err := json.Unmarshal([]byte(redacted), &redactedParsed); err != nil {
		t.Fatalf("redacted json invalid: %v", err)
	}
	if len(redactedParsed.Accounts[0].Credentials) != 0 {
		t.Fatal("expected credentials to be omitted with includeCredentials=false")
	}

	if _, err := manager.ExportAccount("claude", "missing-account", true); err == nil {
		t.Fatal("expected error exporting unknown account")
	}
}
