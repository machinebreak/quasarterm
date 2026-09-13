// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

type SecretBackend interface {
	Set(name string, value string) error
	Get(name string) (string, bool, error)
	Delete(name string) error
}

var secretBackend SecretBackend

func SetSecretBackend(backend SecretBackend) {
	secretBackend = backend
}
