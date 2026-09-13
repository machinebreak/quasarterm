// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"context"
	"fmt"
)

type stubProvider struct {
	info ProviderInfo
}

func (s *stubProvider) Info() ProviderInfo { return s.info }

func (s *stubProvider) unsupported() error {
	return fmt.Errorf("%s support is coming soon", s.info.Name)
}

func (s *stubProvider) CurrentCredentialsPath() (string, error)         { return "", s.unsupported() }
func (s *stubProvider) ImportCurrent() (*Account, []byte, error)        { return nil, nil, s.unsupported() }
func (s *stubProvider) SwitchAccount(*Account, []byte) error            { return s.unsupported() }
func (s *stubProvider) RemoteKey([]byte) (string, error)                { return "", s.unsupported() }
func (s *stubProvider) RefreshQuota(context.Context, *Account, []byte) (*Quota, []byte, error) {
	return nil, nil, s.unsupported()
}

var catalogStubInfos = []ProviderInfo{
}
