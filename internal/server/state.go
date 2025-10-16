package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

type stateCookie struct {
	State    string `json:"state"`
	Redirect string `json:"redirect"`
}

func encryptState(key []byte, state, redirect string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("could not create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("could not create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("could not generate nonce: %w", err)
	}

	data := stateCookie{
		State:    state,
		Redirect: redirect,
	}
	
	plaintext, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("could not marshal state data: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func decryptState(key []byte, encryptedData string) (string, string, error) {
	ciphertext, err := base64.RawURLEncoding.DecodeString(encryptedData)
	if err != nil {
		return "", "", fmt.Errorf("could not decode base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", fmt.Errorf("could not create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", fmt.Errorf("could not create GCM: %w", err)
	}

	if len(ciphertext) < aesGCM.NonceSize() {
		return "", "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:aesGCM.NonceSize()], ciphertext[aesGCM.NonceSize():]

	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", "", fmt.Errorf("could not decrypt: %w", err)
	}

	var data stateCookie
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return "", "", fmt.Errorf("could not unmarshal state data: %w", err)
	}

	return data.State, data.Redirect, nil
}
