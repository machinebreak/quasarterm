// Copyright 2026, Orion
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

var storeLock sync.Mutex

func accountsDir() string {
	dir := filepath.Join(wavebase.GetWaveDataDir(), "accounts")
	os.MkdirAll(dir, 0700)
	return dir
}

func indexFilePath() string {
	return filepath.Join(accountsDir(), "accounts.json")
}

func credentialsDir(provider string) string {
	dir := filepath.Join(accountsDir(), "credentials", provider)
	os.MkdirAll(dir, 0700)
	return dir
}

func credentialsPath(provider string, accountId string) string {
	return filepath.Join(credentialsDir(provider), accountId+".json")
}

func loadIndex() (*indexFile, error) {
	storeLock.Lock()
	defer storeLock.Unlock()
	return loadIndexLocked()
}

func loadIndexLocked() (*indexFile, error) {
	data, err := os.ReadFile(indexFilePath())
	if os.IsNotExist(err) {
		return &indexFile{Version: 1, Accounts: []Account{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error reading accounts index: %w", err)
	}
	var idx indexFile
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("error parsing accounts index: %w", err)
	}
	if idx.Accounts == nil {
		idx.Accounts = []Account{}
	}
	if idx.AutoSwitch == nil {
		idx.AutoSwitch = map[string]bool{}
	}
	return &idx, nil
}

func saveIndex(idx *indexFile) error {
	storeLock.Lock()
	defer storeLock.Unlock()
	return saveIndexLocked(idx)
}

func saveIndexLocked(idx *indexFile) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling accounts index: %w", err)
	}
	return atomicWriteFile(indexFilePath(), data, 0600)
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return err
	}
	os.Remove(path)
	return os.Rename(tmpPath, path)
}

func saveCredentials(provider string, accountId string, data []byte) error {
	if secretBackend != nil {
		if err := secretBackend.Set(credentialsSecretName(provider, accountId), string(data)); err == nil {
			os.Remove(credentialsPath(provider, accountId))
			return nil
		}
	}
	return atomicWriteFile(credentialsPath(provider, accountId), data, 0600)
}

func loadCredentials(provider string, accountId string) ([]byte, error) {
	if secretBackend != nil {
		if value, found, err := secretBackend.Get(credentialsSecretName(provider, accountId)); err == nil && found && value != "" {
			return []byte(value), nil
		}
	}
	data, err := os.ReadFile(credentialsPath(provider, accountId))
	if err != nil {
		return nil, fmt.Errorf("error reading credentials for account %s: %w", accountId, err)
	}
	if secretBackend != nil {
		if err := secretBackend.Set(credentialsSecretName(provider, accountId), string(data)); err == nil {
			os.Remove(credentialsPath(provider, accountId))
		}
	}
	return data, nil
}

func deleteCredentials(provider string, accountId string) {
	if secretBackend != nil {
		secretBackend.Delete(credentialsSecretName(provider, accountId))
	}
	os.Remove(credentialsPath(provider, accountId))
}

func credentialsSecretName(provider string, accountId string) string {
	sanitize := func(value string) string {
		var builder strings.Builder
		for _, char := range value {
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' {
				builder.WriteRune(char)
			} else {
				builder.WriteRune('_')
			}
		}
		return builder.String()
	}
	return "quasar_account_" + sanitize(provider) + "_" + sanitize(accountId)
}

func makeAccountID(provider string, key string) string {
	sum := sha256.Sum256([]byte(provider + "|" + key))
	return hex.EncodeToString(sum[:])[:16]
}

func hashKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}
