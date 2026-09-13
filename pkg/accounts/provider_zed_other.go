//go:build !windows

// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import "fmt"

func readZedCredential() (string, string, error) {
	return "", "", fmt.Errorf("Zed account switching is only supported on Windows")
}

func writeZedCredential(userID string, accessToken string) error {
	return fmt.Errorf("Zed account switching is only supported on Windows")
}
