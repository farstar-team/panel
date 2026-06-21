package security

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVaultRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "master.key")
	vault, err := LoadOrCreateVault(path)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := vault.Encrypt("a very secret tunnel token")
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "a very secret tunnel token" {
		t.Fatal("secret was not encrypted")
	}
	plain, err := vault.Decrypt(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "a very secret tunnel token" {
		t.Fatalf("unexpected plaintext: %q", plain)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("master key permissions = %o", info.Mode().Perm())
	}
}

func TestPassword(t *testing.T) {
	hash, err := HashPassword("a-strong-password")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "a-strong-password") {
		t.Fatal("password verification failed")
	}
	if VerifyPassword(hash, "wrong-password") {
		t.Fatal("wrong password was accepted")
	}
}
