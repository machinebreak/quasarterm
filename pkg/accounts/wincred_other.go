//go:build !windows

// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import "fmt"

func winCredReadTarget(target string) (string, string, error) {
	return "", "", fmt.Errorf("system credential storage is only supported on Windows")
}

func winCredWriteTarget(target string, userName string, secret string) error {
	return fmt.Errorf("system credential storage is only supported on Windows")
}
