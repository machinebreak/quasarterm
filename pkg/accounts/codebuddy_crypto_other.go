//go:build !windows

// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import "fmt"

func decodeCodeBuddySecretValue(rawValue string, dataRoot string) (string, error) {
	return "", fmt.Errorf("CodeBuddy secret storage is only supported on Windows")
}

func encodeCodeBuddySecretValue(plaintext string, dataRoot string) (string, error) {
	return "", fmt.Errorf("CodeBuddy secret storage is only supported on Windows")
}
