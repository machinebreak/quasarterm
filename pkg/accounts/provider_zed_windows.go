//go:build windows

// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	zedCredTypeGeneric      = 1
	zedCredPersistLocalMachine = 2
)

var (
	modAdvapi32    = windows.NewLazySystemDLL("advapi32.dll")
	procCredReadW  = modAdvapi32.NewProc("CredReadW")
	procCredWriteW = modAdvapi32.NewProc("CredWriteW")
	procCredFree   = modAdvapi32.NewProc("CredFree")
)

type zedWindowsCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func zedCredentialTarget() string {
	return "zed:url=https://zed.dev"
}

func readZedCredential() (string, string, error) {
	target, err := windows.UTF16PtrFromString(zedCredentialTarget())
	if err != nil {
		return "", "", err
	}
	var credential *zedWindowsCredential
	ret, _, _ := procCredReadW.Call(
		uintptr(unsafe.Pointer(target)),
		uintptr(zedCredTypeGeneric),
		0,
		uintptr(unsafe.Pointer(&credential)),
	)
	if ret == 0 {
		return "", "", fmt.Errorf("Zed credentials not found in Windows Credential Manager (log in to Zed first)")
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(credential)))
	if credential.UserName == nil || credential.CredentialBlob == nil || credential.CredentialBlobSize == 0 {
		return "", "", fmt.Errorf("Zed credential entry is incomplete")
	}
	userID := windows.UTF16PtrToString(credential.UserName)
	tokenBytes := unsafe.Slice(credential.CredentialBlob, credential.CredentialBlobSize)
	accessToken := string(tokenBytes)
	if userID == "" || accessToken == "" {
		return "", "", fmt.Errorf("Zed credential entry is empty")
	}
	return userID, accessToken, nil
}

func writeZedCredential(userID string, accessToken string) error {
	target, err := windows.UTF16PtrFromString(zedCredentialTarget())
	if err != nil {
		return err
	}
	user, err := windows.UTF16PtrFromString(userID)
	if err != nil {
		return err
	}
	secret := []byte(accessToken)
	credential := zedWindowsCredential{
		Type:               zedCredTypeGeneric,
		TargetName:         target,
		CredentialBlobSize: uint32(len(secret)),
		CredentialBlob:     &secret[0],
		Persist:            zedCredPersistLocalMachine,
		UserName:           user,
	}
	if ret, _, _ := procCredWriteW.Call(uintptr(unsafe.Pointer(&credential)), 0); ret == 0 {
		return fmt.Errorf("error writing Zed credentials to Windows Credential Manager: %v", windows.GetLastError())
	}
	return nil
}
