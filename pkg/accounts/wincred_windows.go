//go:build windows

// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const winCredTypeGeneric = 1
const winCredPersistLocalMachine = 2

var (
	advapiCredRead  = windows.NewLazySystemDLL("advapi32.dll").NewProc("CredReadW")
	advapiCredWrite = windows.NewLazySystemDLL("advapi32.dll").NewProc("CredWriteW")
	advapiCredFree  = windows.NewLazySystemDLL("advapi32.dll").NewProc("CredFree")
)

type winCredential struct {
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

func winCredReadTarget(target string) (string, string, error) {
	targetPtr, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return "", "", err
	}
	var credential *winCredential
	ret, _, _ := advapiCredRead.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		uintptr(winCredTypeGeneric),
		0,
		uintptr(unsafe.Pointer(&credential)),
	)
	if ret == 0 {
		return "", "", fmt.Errorf("credentials not found in Windows Credential Manager (target=%s)", target)
	}
	defer advapiCredFree.Call(uintptr(unsafe.Pointer(credential)))
	userName := ""
	if credential.UserName != nil {
		userName = windows.UTF16PtrToString(credential.UserName)
	}
	if credential.CredentialBlob == nil || credential.CredentialBlobSize == 0 {
		return userName, "", nil
	}
	secret := string(unsafe.Slice(credential.CredentialBlob, credential.CredentialBlobSize))
	return userName, secret, nil
}

func winCredWriteTarget(target string, userName string, secret string) error {
	targetPtr, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	userPtr, err := windows.UTF16PtrFromString(userName)
	if err != nil {
		return err
	}
	secretBytes := []byte(secret)
	credential := winCredential{
		Type:               winCredTypeGeneric,
		TargetName:         targetPtr,
		CredentialBlobSize: uint32(len(secretBytes)),
		CredentialBlob:     &secretBytes[0],
		Persist:            winCredPersistLocalMachine,
		UserName:           userPtr,
	}
	if ret, _, _ := advapiCredWrite.Call(uintptr(unsafe.Pointer(&credential)), 0); ret == 0 {
		return fmt.Errorf("error writing credentials to Windows Credential Manager: %v", windows.GetLastError())
	}
	return nil
}
