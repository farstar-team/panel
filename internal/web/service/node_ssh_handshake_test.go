package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/mhsanaei/3x-ui/v3/internal/crypto/nodetoken"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestNodeSSHChangedHostKeyNeverReceivesPassword(t *testing.T) {
	enableNodeTokenEncryption(t)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	var authentications atomic.Int32
	config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		authentications.Add(1)
		if string(password) != "node-private-password" {
			return nil, errors.New("invalid password")
		}
		return nil, nil
	}}
	config.AddHostKey(signer)
	additionalKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	additionalSigner, err := ssh.NewSignerFromKey(additionalKey)
	if err != nil {
		t.Fatal(err)
	}
	config.AddHostKey(additionalSigner)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				server, channels, requests, err := ssh.NewServerConn(connection, config)
				if err != nil {
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					_ = channel.Reject(ssh.Prohibited, "test server")
				}
			}()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	encrypted, err := nodetoken.EncryptBound(sshBinding(12, "password"), "node-private-password")
	if err != nil {
		t.Fatal(err)
	}
	node := &model.Node{Id: 12, Address: host, SSHPort: port, SSHUsername: "root", SSHPassword: encrypted, SSHFingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", AllowPrivateAddress: true}
	client, err := dialNodeSSH(ctx, node)
	if client != nil {
		client.Close()
		t.Fatal("connection accepted an untrusted host key")
	}
	if err == nil || !strings.Contains(err.Error(), "SSH host key changed; connection refused") {
		t.Fatalf("wrong-key error = %v", err)
	}
	if authentications.Load() != 0 {
		t.Fatal("password sent before host verification")
	}
	observed, err := (&NodeService{}).SSHFingerprint(ctx, &NodeSSHProbeRequest{Address: host, Port: port, AllowPrivateAddress: true})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Fingerprint != ssh.FingerprintSHA256(signer.PublicKey()) {
		t.Fatal("fingerprint discovery did not prefer the pinned Ed25519 host key")
	}
	if authentications.Load() != 0 {
		t.Fatal("fingerprint discovery authenticated instead of stopping before credentials")
	}
	node.SSHFingerprint = observed.Fingerprint
	client, err = dialNodeSSH(ctx, node)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if authentications.Load() != 1 {
		t.Fatalf("trusted host authentication count = %d", authentications.Load())
	}
}
