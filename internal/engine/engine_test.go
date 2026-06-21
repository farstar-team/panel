package engine

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/farstar-team/panel/internal/store"
)

func TestTCPChallengeAuthentication(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	result := make(chan error, 1)
	go func() { result <- authenticateServer(server, "0123456789abcdef-secret") }()
	if err := authenticateClient(client, "0123456789abcdef-secret"); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsWeakSecret(t *testing.T) {
	err := Validate(store.Tunnel{
		Name: "test", Role: "server", Protocol: "tcp",
		ListenAddr: "127.0.0.1:443", PublicPorts: []string{"127.0.0.1:8000"},
	}, "short")
	if err == nil {
		t.Fatal("weak secret was accepted")
	}
}

func TestWSSReverseTunnelWithVerifiedCA(t *testing.T) {
	dir := t.TempDir()
	database, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	certPath, keyPath := writeTestCertificate(t, dir)
	tunnelAddress := reserveAddress(t)
	publicAddress := reserveAddress(t)
	echoListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoListener.Close()
	go func() {
		for {
			conn, err := echoListener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()

	secret := "farstar-wss-integration-secret"
	serverTunnel := store.Tunnel{
		ID: "wss-server", Name: "wss-server", Role: "server", Protocol: "wss",
		ListenAddr: tunnelAddress, PublicPorts: []string{publicAddress},
		TLSCert: certPath, TLSKey: keyPath,
	}
	clientTunnel := store.Tunnel{
		ID: "wss-client", Name: "wss-client", Role: "client", Protocol: "wss",
		RemoteAddr:    "wss://" + tunnelAddress + "/tunnel",
		LocalServices: []string{echoListener.Addr().String()},
		TLSCACert:     certPath,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverDone := make(chan error, 1)
	clientDone := make(chan error, 1)
	go func() {
		serverDone <- (&Runner{
			Store: database, Tunnel: serverTunnel, Secret: secret,
			Logger: log.New(io.Discard, "", 0),
		}).Run(ctx)
	}()
	time.Sleep(100 * time.Millisecond)
	go func() {
		clientDone <- (&Runner{
			Store: database, Tunnel: clientTunnel, Secret: secret,
			Logger: log.New(io.Discard, "", 0),
		}).Run(ctx)
	}()

	deadline := time.Now().Add(8 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", publicAddress, 250*time.Millisecond)
		if err == nil {
			_ = conn.SetDeadline(time.Now().Add(time.Second))
			payload := []byte("verified-wss-path")
			_, writeErr := conn.Write(payload)
			reply := make([]byte, len(payload))
			_, readErr := io.ReadFull(conn, reply)
			_ = conn.Close()
			if writeErr == nil && readErr == nil && string(reply) == string(payload) {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("verified WSS tunnel did not become ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	select {
	case <-serverDone:
	case <-time.After(3 * time.Second):
		t.Fatal("WSS server did not stop")
	}
	select {
	case <-clientDone:
	case <-time.After(3 * time.Second):
		t.Fatal("WSS client did not stop")
	}
}

func reserveAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	return address
}

func writeTestCertificate(t *testing.T, dir string) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	certificate, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}
