package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

type Vault struct {
	aead cipher.AEAD
}

func LoadOrCreateVault(path string) (*Vault, error) {
	key, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generate master key: %w", err)
		}
		if err := os.WriteFile(path, key, 0600); err != nil {
			return nil, fmt.Errorf("write master key: %w", err)
		}
		if err := os.Chmod(path, 0600); err != nil {
			return nil, fmt.Errorf("secure master key permissions: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("master key must be exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: aead}, nil
}

func (v *Vault) Encrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := v.aead.Seal(nonce, nonce, []byte(value), nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (v *Vault) Decrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	sealed, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(sealed) < v.aead.NonceSize() {
		return "", errors.New("invalid encrypted value")
	}
	nonce := sealed[:v.aead.NonceSize()]
	plain, err := v.aead.Open(nil, nonce, sealed[v.aead.NonceSize():], nil)
	if err != nil {
		return "", errors.New("cannot decrypt value")
	}
	return string(plain), nil
}

func HashPassword(password string) (string, error) {
	if len(password) < 10 {
		return "", errors.New("password must contain at least 10 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(hash), err
}

func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func RandomToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func ChallengeMAC(secret string, nonce []byte) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(nonce)
	return mac.Sum(nil)
}

func SecureEqual(a, b []byte) bool {
	return hmac.Equal(a, b)
}
