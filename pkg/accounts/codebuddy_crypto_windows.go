//go:build windows

// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modCrypt32   = windows.NewLazySystemDLL("crypt32.dll")
	modKernel32  = windows.NewLazySystemDLL("kernel32.dll")
	procUnprotect = modCrypt32.NewProc("CryptUnprotectData")
	procLocalFree = modKernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func dpapiDecrypt(encrypted []byte) ([]byte, error) {
	if len(encrypted) == 0 {
		return nil, fmt.Errorf("empty DPAPI blob")
	}
	input := dataBlob{cbData: uint32(len(encrypted)), pbData: &encrypted[0]}
	var output dataBlob
	ret, _, _ := procUnprotect.Call(
		uintptr(unsafe.Pointer(&input)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&output)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("DPAPI CryptUnprotectData failed: %v", windows.GetLastError())
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(output.pbData)))
	result := make([]byte, output.cbData)
	copy(result, unsafe.Slice(output.pbData, output.cbData))
	return result, nil
}

func codebuddyEncryptionKey(dataRoot string) ([]byte, error) {
	localStatePath := filepath.Join(dataRoot, "Local State")
	raw, err := os.ReadFile(localStatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Local State: %w", err)
	}
	var localState map[string]any
	if err := json.Unmarshal(raw, &localState); err != nil {
		return nil, fmt.Errorf("failed to parse Local State: %w", err)
	}
	osCrypt, _ := localState["os_crypt"].(map[string]any)
	encryptedKeyB64, _ := osCrypt["encrypted_key"].(string)
	if encryptedKeyB64 == "" {
		return nil, fmt.Errorf("Local State is missing os_crypt.encrypted_key")
	}
	encryptedKey, err := base64.StdEncoding.DecodeString(encryptedKeyB64)
	if err != nil || len(encryptedKey) < 6 {
		return nil, fmt.Errorf("invalid encrypted_key in Local State")
	}
	if string(encryptedKey[:5]) != "DPAPI" {
		return nil, fmt.Errorf("encrypted_key is not DPAPI protected")
	}
	key, err := dpapiDecrypt(encryptedKey[5:])
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("decrypted AES key has unexpected length %d", len(key))
	}
	return key, nil
}

func decryptCodeBuddyV10(key []byte, encrypted []byte) ([]byte, error) {
	if len(encrypted) < 31 || string(encrypted[:3]) != "v10" {
		return nil, fmt.Errorf("unsupported secret storage payload")
	}
	nonce := encrypted[3:15]
	ciphertext := encrypted[15:]
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func encryptCodeBuddyV10(key []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	result := append([]byte("v10"), nonce...)
	result = append(result, sealed...)
	return result, nil
}

func decodeCodeBuddySecretValue(rawValue string, dataRoot string) (string, error) {
	var parsed any
	if err := json.Unmarshal([]byte(rawValue), &parsed); err != nil {
		return rawValue, nil
	}
	if text, ok := parsed.(string); ok {
		return text, nil
	}
	obj, ok := parsed.(map[string]any)
	if !ok {
		return rawValue, nil
	}
	dataArray, ok := obj["data"].([]any)
	if !ok {
		return rawValue, nil
	}
	encrypted := make([]byte, 0, len(dataArray))
	for _, entry := range dataArray {
		f, ok := jsonFloat(entry)
		if !ok {
			return "", fmt.Errorf("invalid secret storage buffer")
		}
		encrypted = append(encrypted, byte(int(f)&0xff))
	}
	key, err := codebuddyEncryptionKey(dataRoot)
	if err != nil {
		return "", err
	}
	plain, err := decryptCodeBuddyV10(key, encrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt CodeBuddy secret: %w", err)
	}
	return string(plain), nil
}

func encodeCodeBuddySecretValue(plaintext string, dataRoot string) (string, error) {
	key, err := codebuddyEncryptionKey(dataRoot)
	if err != nil {
		return "", err
	}
	encrypted, err := encryptCodeBuddyV10(key, []byte(plaintext))
	if err != nil {
		return "", err
	}
	bytesArray := make([]int, len(encrypted))
	for i, b := range encrypted {
		bytesArray[i] = int(b)
	}
	encoded, err := json.Marshal(map[string]any{"data": bytesArray})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
