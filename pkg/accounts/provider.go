// Copyright 2026, Orion
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"context"
	"net/http"
	"time"
)

type Provider interface {
	Info() ProviderInfo
	CurrentCredentialsPath() (string, error)
	ImportCurrent() (*Account, []byte, error)
	SwitchAccount(account *Account, credentials []byte) error
	RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error)
	RemoteKey(credentials []byte) (string, error)
}

type ActiveKeyProvider interface {
	ActiveRemoteKey() (string, error)
}

type InstanceProvider interface {
	InstancePrepare(account *Account, credentials []byte, profileDir string) (execPath string, args []string, env []string, err error)
}

// WakeProvider is implemented by providers whose CLI can run a minimal
// non-interactive request ("wake") with an account's credentials.
type WakeProvider interface {
	WakeArgs(model string) []string
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

func newRequest(ctx context.Context, method string, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	return req, nil
}
