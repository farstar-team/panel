package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/mhsanaei/3x-ui/v3/internal/crypto/nodetoken"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/netsafe"
)

const sshEncryptionRequired = "SSH credentials require NODE_TOKEN_ENCRYPTION=required and a protected key file"

var sshUserPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,31}$`)

var nodeSSHHostKeyAlgorithms = []string{
	ssh.KeyAlgoED25519,
	ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521,
	ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256,
}

func sshBinding(id int, field string) []byte {
	return []byte(fmt.Sprintf("nodes/ssh/%d/%s", id, field))
}

type NodeSSHRequest struct {
	Port           int     `json:"port"`
	Username       string  `json:"username"`
	Fingerprint    string  `json:"fingerprint"`
	Password       *string `json:"password,omitempty"`
	PrivateKey     *string `json:"privateKey,omitempty"`
	TrustConfirmed bool    `json:"trustConfirmed"`
}

func validateNodeSSH(req *NodeSSHRequest, existing *model.Node, required bool) error {
	stored := existing != nil && (existing.SSHPassword != "" || existing.SSHPrivateKey != "")
	if req == nil {
		if required && !stored {
			return errors.New("SSH credentials are required for an Iran node")
		}
		return nil
	}
	if !nodetoken.Enabled() {
		return errors.New(sshEncryptionRequired)
	}
	if req.Password != nil && *req.Password == "" {
		req.Password = nil
	}
	if req.Password != nil && len(*req.Password) > 4096 {
		return errors.New("SSH password is too long")
	}
	if req.PrivateKey != nil && len(*req.PrivateKey) > 32768 {
		return errors.New("SSH private key is too large")
	}
	if req.PrivateKey != nil && strings.TrimSpace(*req.PrivateKey) == "" {
		req.PrivateKey = nil
	}
	if req.Port < 1 || req.Port > 65535 || !sshUserPattern.MatchString(req.Username) {
		return errors.New("invalid SSH port or username")
	}
	encoded := strings.TrimPrefix(req.Fingerprint, "SHA256:")
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != 32 || !strings.HasPrefix(req.Fingerprint, "SHA256:") {
		return errors.New("SSH host fingerprint must be SHA256")
	}
	if req.Password != nil && req.PrivateKey != nil {
		return errors.New("choose SSH password or private key, not both")
	}
	if !stored && req.Password == nil && req.PrivateKey == nil {
		return errors.New("SSH credentials are required")
	}
	if req.PrivateKey != nil {
		if _, err := ssh.ParsePrivateKey([]byte(*req.PrivateKey)); err != nil {
			return errors.New("SSH private key must be valid and unencrypted")
		}
	}
	changed := existing == nil || req.Fingerprint != existing.SSHFingerprint || req.Port != existing.SSHPort || req.Username != existing.SSHUsername || req.Password != nil || req.PrivateKey != nil
	if changed && !req.TrustConfirmed {
		return errors.New("verify the SSH fingerprint against the server console before saving")
	}
	return nil
}

func addSSHUpdates(updates map[string]any, id int, req *NodeSSHRequest) error {
	if req == nil {
		return nil
	}
	updates["ssh_port"], updates["ssh_username"], updates["ssh_fingerprint"] = req.Port, req.Username, req.Fingerprint
	if req.Password != nil {
		encrypted, err := nodetoken.EncryptBound(sshBinding(id, "password"), *req.Password)
		if err != nil {
			return err
		}
		updates["ssh_password"], updates["ssh_private_key"] = encrypted, ""
	}
	if req.PrivateKey != nil {
		encrypted, err := nodetoken.EncryptBound(sshBinding(id, "private-key"), *req.PrivateKey)
		if err != nil {
			return err
		}
		updates["ssh_private_key"], updates["ssh_password"] = encrypted, ""
	}
	return nil
}

type NodeSSHProbeRequest struct {
	Address             string `json:"address" validate:"required"`
	Port                int    `json:"port" validate:"gte=1,lte=65535"`
	AllowPrivateAddress bool   `json:"allowPrivateAddress"`
}
type NodeSSHFingerprint struct {
	Fingerprint string `json:"fingerprint"`
	KeyType     string `json:"keyType"`
}

func (s *NodeService) SSHFingerprint(ctx context.Context, req *NodeSSHProbeRequest) (*NodeSSHFingerprint, error) {
	host, err := netsafe.NormalizeHost(req.Address)
	if err != nil {
		return nil, err
	}
	if req.Port < 1 || req.Port > 65535 {
		return nil, errors.New("invalid SSH port")
	}
	ctx, cancel := context.WithTimeout(netsafe.ContextWithAllowPrivate(ctx, req.AllowPrivateAddress), 15*time.Second)
	defer cancel()
	address := net.JoinHostPort(host, strconv.Itoa(req.Port))
	conn, err := netsafe.SSRFGuardedDialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	var observed *NodeSSHFingerprint
	_, _, _, handshakeErr := ssh.NewClientConn(conn, address, &ssh.ClientConfig{User: "fingerprint-probe", HostKeyAlgorithms: nodeSSHHostKeyAlgorithms, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		observed = &NodeSSHFingerprint{Fingerprint: ssh.FingerprintSHA256(key), KeyType: key.Type()}
		return errors.New("fingerprint observed; authentication deliberately stopped")
	}})
	if observed == nil {
		return nil, handshakeErr
	}
	return observed, nil
}

func dialNodeSSH(ctx context.Context, node *model.Node) (*ssh.Client, error) {
	if node.SSHFingerprint == "" {
		return nil, errors.New("SSH host fingerprint is required")
	}
	password, err := nodetoken.DecryptBound(sshBinding(node.Id, "password"), node.SSHPassword)
	if err != nil {
		return nil, err
	}
	key, err := nodetoken.DecryptBound(sshBinding(node.Id, "private-key"), node.SSHPrivateKey)
	if err != nil {
		return nil, err
	}
	var auth ssh.AuthMethod
	if key != "" {
		signer, err := ssh.ParsePrivateKey([]byte(key))
		if err != nil {
			return nil, errors.New("invalid stored SSH private key")
		}
		auth = ssh.PublicKeys(signer)
	} else if password != "" {
		auth = ssh.Password(password)
	} else {
		return nil, errors.New("SSH credentials missing")
	}
	host, err := netsafe.NormalizeHost(node.Address)
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(host, strconv.Itoa(node.SSHPort))
	conn, err := netsafe.SSRFGuardedDialContext(netsafe.ContextWithAllowPrivate(ctx, node.AllowPrivateAddress), "tcp", address)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(20 * time.Second)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	_ = conn.SetDeadline(deadline)
	config := &ssh.ClientConfig{User: node.SSHUsername, Auth: []ssh.AuthMethod{auth}, HostKeyAlgorithms: nodeSSHHostKeyAlgorithms, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if ssh.FingerprintSHA256(key) != node.SSHFingerprint {
			return errors.New("SSH host key changed; connection refused")
		}
		return nil
	}}
	connection, channels, requests, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	client := ssh.NewClient(connection, channels, requests)
	go func() { <-ctx.Done(); _ = client.Close() }()
	return client, nil
}
