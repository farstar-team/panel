package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/farstar-team/panel/internal/config"
	"github.com/farstar-team/panel/internal/manager"
	"github.com/farstar-team/panel/internal/security"
	"github.com/farstar-team/panel/internal/store"
)

func TestAuthenticatedTunnelAPI(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		DataDir: dir, DBPath: filepath.Join(dir, "test.db"),
		LogDir: filepath.Join(dir, "logs"), MasterKey: filepath.Join(dir, "master.key"),
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	hash, _ := security.HashPassword("integration-password")
	if err := database.CreateAdmin(context.Background(), "admin", hash); err != nil {
		t.Fatal(err)
	}
	vault, err := security.LoadOrCreateVault(cfg.MasterKey)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(database, vault, manager.New(database, cfg), cfg).Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "integration-password"})
	response, err := client.Post(server.URL+"/api/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	var login struct {
		CSRF string `json:"csrf"`
	}
	if err := json.NewDecoder(response.Body).Decode(&login); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || login.CSRF == "" {
		t.Fatalf("login status=%d csrf=%q", response.StatusCode, login.CSRF)
	}

	tunnelBody, _ := json.Marshal(map[string]any{
		"name": "test-server", "role": "server", "protocol": "tcp",
		"listen_addr": "127.0.0.1:19443", "remote_addr": "",
		"public_ports": []string{"127.0.0.1:19800"}, "local_services": []string{},
		"secret": "0123456789abcdef-secret", "tls_cert": "", "tls_key": "",
		"tls_server_name": "", "tls_ca_cert": "", "skip_tls_verify": false, "autostart": true,
	})
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/tunnels", bytes.NewReader(tunnelBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", login.CSRF)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var created store.Tunnel
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d", response.StatusCode)
	}
	if created.ID == "" || created.Secret != "" {
		t.Fatalf("unexpected API tunnel: %+v", created)
	}

	response, err = client.Get(server.URL + "/api/tunnels")
	if err != nil {
		t.Fatal(err)
	}
	var tunnels []store.Tunnel
	if err := json.NewDecoder(response.Body).Decode(&tunnels); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if len(tunnels) != 1 || tunnels[0].Name != "test-server" {
		t.Fatalf("unexpected tunnel list: %+v", tunnels)
	}
}

func TestMutationRequiresCSRF(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		DataDir: dir, DBPath: filepath.Join(dir, "test.db"),
		LogDir: filepath.Join(dir, "logs"), MasterKey: filepath.Join(dir, "master.key"),
	}
	_ = cfg.EnsureDirs()
	database, _ := store.Open(cfg.DBPath)
	defer database.Close()
	hash, _ := security.HashPassword("integration-password")
	_ = database.CreateAdmin(context.Background(), "admin", hash)
	vault, _ := security.LoadOrCreateVault(cfg.MasterKey)
	server := httptest.NewServer(New(database, vault, manager.New(database, cfg), cfg).Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "integration-password"})
	response, _ := client.Post(server.URL+"/api/auth/login", "application/json", bytes.NewReader(loginBody))
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/tunnels", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("mutation without CSRF returned %d", response.StatusCode)
	}
}
