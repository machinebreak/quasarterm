// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

var traePrefixAES = []byte{116, 99, 5, 16, 0, 0}
var traePrefixAESPrivate = []byte{18, 57, 32, 32, 2, 3}

var traeAESPrivateA = [64]byte{
	191, 192, 216, 250, 122, 246, 220, 97, 31, 254, 98, 27, 8, 72, 71, 176, 135, 99, 96, 18, 127,
	101, 203, 104, 211, 102, 191, 125, 37, 72, 150, 156, 51, 229, 121, 35, 17, 153, 141, 177, 110,
	131, 150, 128, 172, 255, 254, 6, 18, 140, 55, 62, 236, 249, 135, 64, 135, 12, 117, 4, 89, 149,
	168, 209,
}

var traeAESPrivateB = [64]byte{
	246, 204, 26, 232, 232, 70, 129, 109, 223, 146, 169, 242, 23, 241, 105, 145, 50, 196, 165, 42,
	254, 120, 3, 54, 244, 207, 209, 85, 53, 6, 138, 106, 175, 148, 31, 204, 186, 186, 165, 182, 87,
	142, 49, 10, 39, 110, 26, 154, 86, 56, 173, 125, 18, 64, 198, 225, 99, 99, 83, 82, 191, 134,
	76, 170,
}

var traeAESA = [64]byte{
	82, 9, 106, 213, 48, 54, 165, 56, 191, 64, 163, 158, 129, 243, 215, 251, 124, 227, 57, 130,
	155, 47, 255, 135, 52, 142, 67, 68, 196, 222, 233, 203, 84, 123, 148, 50, 166, 194, 35, 61,
	238, 76, 149, 11, 66, 250, 195, 78, 8, 46, 161, 102, 40, 217, 36, 178, 118, 91, 162, 73, 109,
	139, 209, 37,
}

var traeAESB = [64]byte{
	31, 221, 168, 51, 136, 7, 199, 49, 177, 18, 16, 89, 39, 128, 236, 95, 96, 81, 127, 169, 25,
	181, 74, 13, 45, 229, 122, 159, 147, 201, 156, 239, 160, 224, 59, 77, 174, 42, 245, 176, 200,
	235, 187, 60, 131, 83, 153, 97, 23, 43, 4, 126, 186, 119, 214, 38, 225, 105, 20, 99, 85, 33,
	12, 125,
}

type traeCryptoVersion int

const (
	traeCryptoAES traeCryptoVersion = iota + 1
	traeCryptoAESPrivate
)

func traeVersionFromHeader(header []byte) (traeCryptoVersion, bool) {
	if bytes.Equal(header, traePrefixAES) {
		return traeCryptoAES, true
	}
	if bytes.Equal(header, traePrefixAESPrivate) {
		return traeCryptoAESPrivate, true
	}
	return 0, false
}

func traeCryptoSalt(version traeCryptoVersion) [64]byte {
	left := traeAESA
	right := traeAESB
	if version == traeCryptoAESPrivate {
		left = traeAESPrivateA
		right = traeAESPrivateB
	}
	var salt [64]byte
	for i := 0; i < 64; i++ {
		salt[i] = left[i] ^ right[i]
	}
	return salt
}

func traeDeriveKeyIV(keyMaterial []byte, version traeCryptoVersion) ([]byte, []byte, error) {
	if len(keyMaterial) != 32 {
		return nil, nil, fmt.Errorf("invalid Trae key material length")
	}
	keyHash := sha512.Sum512(keyMaterial)
	merge := make([]byte, 0, 128)
	merge = append(merge, keyHash[:]...)
	salt := traeCryptoSalt(version)
	merge = append(merge, salt[:]...)
	merged := sha512.Sum512(merge)
	return merged[:16], merged[16:32], nil
}

func traeByteCryptoEncrypt(plaintext []byte) ([]byte, error) {
	randomKey := make([]byte, 32)
	if _, err := rand.Read(randomKey); err != nil {
		return nil, err
	}
	key, iv, err := traeDeriveKeyIV(randomKey, traeCryptoAES)
	if err != nil {
		return nil, err
	}
	digest := sha512.Sum512(plaintext)
	payload := make([]byte, 0, 64+len(plaintext))
	payload = append(payload, digest[:]...)
	payload = append(payload, plaintext...)
	padLen := 16 - (len(payload) % 16)
	padded := append(payload, make([]byte, padLen)...)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)
	out := make([]byte, 0, 6+32+len(encrypted))
	out = append(out, traePrefixAES...)
	out = append(out, randomKey...)
	out = append(out, encrypted...)
	return out, nil
}

func traeByteCryptoDecrypt(raw []byte) ([]byte, error) {
	if len(raw) <= 6+32 {
		return nil, fmt.Errorf("Trae ciphertext too short")
	}
	version, ok := traeVersionFromHeader(raw[:6])
	if !ok {
		return nil, fmt.Errorf("unsupported Trae ciphertext header")
	}
	keyMaterial := raw[6:38]
	ciphertext := raw[38:]
	if len(ciphertext) == 0 || len(ciphertext)%16 != 0 {
		return nil, fmt.Errorf("invalid Trae ciphertext length")
	}
	key, iv, err := traeDeriveKeyIV(keyMaterial, version)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	decrypted := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(decrypted, ciphertext)
	decrypted = traePKCS7Unpad(decrypted)
	if decrypted == nil || len(decrypted) < 64 {
		return nil, fmt.Errorf("invalid Trae plaintext")
	}
	digest := sha512.Sum512(decrypted[64:])
	if !bytes.Equal(digest[:], decrypted[:64]) {
		return nil, fmt.Errorf("Trae integrity check failed")
	}
	return decrypted[64:], nil
}

func traePKCS7Unpad(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > 16 || pad > len(data) {
		return nil
	}
	for i := len(data) - pad; i < len(data); i++ {
		if int(data[i]) != pad {
			return nil
		}
	}
	return data[:len(data)-pad]
}

func traeDecodeStorageValue(value any) (any, error) {
	switch typed := value.(type) {
	case map[string]any, []any:
		return typed, nil
	case string:
		trimmed := trimJSONString(typed)
		if trimmed == "" {
			return nil, fmt.Errorf("empty Trae storage value")
		}
		var parsed any
		if json.Unmarshal([]byte(trimmed), &parsed) == nil {
			return parsed, nil
		}
		decoded, err := base64.StdEncoding.DecodeString(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid Trae base64 value: %w", err)
		}
		plain, err := traeByteCryptoDecrypt(decoded)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(plain, &parsed); err != nil {
			return nil, fmt.Errorf("invalid Trae decrypted JSON: %w", err)
		}
		return parsed, nil
	}
	return nil, fmt.Errorf("unsupported Trae storage value")
}

func traeEncodeStorageValue(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	encrypted, err := traeByteCryptoEncrypt(encoded)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}
