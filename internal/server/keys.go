package server

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

func loadSigningKey(path string) (*rsa.PrivateKey, error) {
	pemData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing RSA private key")
	}

	if block.Type != "RSA PRIVATE KEY" {
		return nil, fmt.Errorf("invalid PEM block type: expected RSA PRIVATE KEY, got %s", block.Type)
	}

	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return privKey, nil
}
