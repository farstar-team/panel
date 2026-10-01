package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/crypto/nodetoken"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

var (
	provisioningGate      = make(chan struct{}, 1)
	provisioningMutex     sync.Mutex
	provisionTokenPattern = regexp.MustCompile(`(?m)^apiToken: ([A-Za-z0-9_-]{16,256})\s*$`)
)

func guardNodeProvisioning(node *model.Node) error {
	if node.ProvisionStatus == "pending" || node.ProvisionStatus == "running" {
		return errors.New("automatic installation is running; wait for it to finish before changing or deleting this node")
	}
	return nil
}

func guardNodeProvisioningByID(id int) error {
	var node model.Node
	if err := database.GetDB().Select("provision_status").First(&node, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	return guardNodeProvisioning(&node)
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func (s *NodeService) StartAutomaticInstall(req *NodeMutationRequest) (*NodeView, error) {
	if req == nil || !req.AutoInstall {
		return nil, errors.New("automatic installation must be selected explicitly")
	}
	if !nodetoken.Enabled() {
		return nil, errors.New(sshEncryptionRequired)
	}
	if err := validateNodeSSH(req.SSH, nil, true); err != nil {
		return nil, err
	}
	n := req.toNode()
	n.ApiToken = ""
	n.Enable = false
	n.Scheme = "https"
	n.TlsVerifyMode = "verify"
	n.PinnedCertSha256 = ""
	if err := s.normalize(n); err != nil {
		return nil, err
	}
	if n.BasePath == "/" {
		return nil, errors.New("choose a non-root, unpredictable panel base path for the new node")
	}
	provisioningMutex.Lock()
	defer provisioningMutex.Unlock()
	select {
	case provisioningGate <- struct{}{}:
	default:
		return nil, errors.New("another automatic installation is running; try again after it finishes")
	}
	release := true
	defer func() {
		if release {
			<-provisioningGate
		}
	}()
	var existingCount int64
	if err := database.GetDB().Model(&model.Node{}).Where("address = ?", n.Address).Count(&existingCount).Error; err != nil {
		return nil, err
	}
	if existingCount > 0 {
		return nil, errors.New("this server is already registered; automatic installation is fresh-server only")
	}
	n.ProvisionStatus = "pending"
	if err := s.Create(n); err != nil {
		return nil, err
	}
	release = false
	go func() { defer func() { <-provisioningGate }(); s.installFreshNode(n.Id) }()
	return toNodeView(n), nil
}

func (s *NodeService) InterruptPendingInstallations() error {
	return database.GetDB().Model(&model.Node{}).Where("provision_status IN ?", []string{"pending", "running"}).Updates(map[string]any{"provision_status": "failed", "provision_error": "Panel restarted during installation. Inspect the remote server before retrying.", "enable": false}).Error
}

func (s *NodeService) installFreshNode(id int) {
	db := database.GetDB()
	_ = db.Model(&model.Node{}).Where("id = ?", id).Updates(map[string]any{"provision_status": "running", "provision_error": ""}).Error
	failed := func(message string) {
		_ = db.Model(&model.Node{}).Where("id = ?", id).Updates(map[string]any{"provision_status": "failed", "provision_error": message, "enable": false}).Error
	}
	var node model.Node
	if err := db.First(&node, id).Error; err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	client, err := dialNodeSSH(ctx, &node)
	if err != nil {
		failed("SSH connection failed. Check reachability, credentials and the trusted host fingerprint.")
		return
	}
	defer client.Close()
	if _, err := sshRun(client, "test \"$(id -u)\" = 0 && test \"$(uname -s)\" = Linux && command -v systemctl && ! test -e /usr/local/x-ui && ! test -e /etc/x-ui/x-ui.db", nil); err != nil {
		failed("Fresh Linux/systemd server with root SSH is required. Existing panel/database will not be overwritten.")
		return
	}
	script := freshNodeScript(&node)
	if _, err := sshRun(client, "bash -s", strings.NewReader(script)); err != nil {
		failed("Installation failed. Inspect /var/log/farstar-node-install.log on the node; existing data was not overwritten.")
		return
	}
	certificate, err := sshRun(client, "cat /etc/x-ui/farstar-node/cert.pem", nil)
	if err != nil {
		failed("Could not read the node TLS certificate through trusted SSH.")
		return
	}
	block, _ := pem.Decode(certificate)
	if block == nil {
		failed("Invalid node TLS certificate.")
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		failed("Invalid node TLS certificate.")
		return
	}
	sum := sha256.Sum256(cert.Raw)
	node.PinnedCertSha256 = base64.StdEncoding.EncodeToString(sum[:])
	node.TlsVerifyMode = "pin"
	tokenOutput, err := sshRun(client, "/usr/local/x-ui/x-ui setting -getApiToken -tokenName FARSTAR-master", nil)
	if err != nil {
		failed("Could not create the node API credential.")
		return
	}
	matches := provisionTokenPattern.FindSubmatch(tokenOutput)
	if len(matches) != 2 {
		failed("Node API credential was not returned in the expected format.")
		return
	}
	if _, err := sshRun(client, `sqlite3 /etc/x-ui/x-ui.db "UPDATE api_tokens SET scope='node-sync' WHERE name='FARSTAR-master';"`, nil); err != nil {
		failed("Could not restrict the node API credential to node-sync scope.")
		return
	}
	node.ApiToken = string(matches[1])
	encrypted, err := nodetoken.Encrypt(id, node.ApiToken)
	if err != nil {
		failed("Could not encrypt the node credential.")
		return
	}
	if err := db.Model(&model.Node{}).Where("id = ?", id).Updates(map[string]any{"api_token": encrypted, "tls_verify_mode": "pin", "pinned_cert_sha256": node.PinnedCertSha256, "enable": false}).Error; err != nil {
		failed("Installed, but registration could not be saved.")
		return
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 15*time.Second)
	defer probeCancel()
	if _, err := s.Probe(probeCtx, &node); err != nil {
		failed("Installed, but the pinned HTTPS API is unreachable. Check the node's firewall and selected panel port.")
		return
	}
	if err := db.Model(&model.Node{}).Where("id = ?", id).Updates(map[string]any{"api_token": encrypted, "tls_verify_mode": "pin", "pinned_cert_sha256": node.PinnedCertSha256, "enable": true, "provision_status": "ready", "provision_error": ""}).Error; err != nil {
		failed("Installed, but registration could not be saved.")
		return
	}
	if manager := runtime.GetManager(); manager != nil {
		manager.InvalidateNode(id)
	}
}

func freshNodeScript(node *model.Node) string {
	return fmt.Sprintf(`set -euo pipefail
umask 077
exec 9>/run/farstar-provision.lock
flock -n 9
test "$(id -u)" = 0
test "$(uname -s)" = Linux
command -v systemctl >/dev/null
. /etc/os-release
case "$ID" in ubuntu|debian) ;; *) exit 75 ;; esac
if test -e /usr/local/x-ui || test -e /etc/x-ui/x-ui.db; then exit 73; fi
if ss -Hln sport = :%d | grep -q .; then exit 74; fi
command -v curl >/dev/null || { apt-get update >/var/log/farstar-node-packages.log 2>&1; apt-get install -y curl ca-certificates >>/var/log/farstar-node-packages.log 2>&1; }
installer=$(mktemp /tmp/farstar-installer.XXXXXX)
trap 'rm -f "$installer"' EXIT
curl -fsSL --max-time 60 https://raw.githubusercontent.com/farstar-team/panel/0bb4659795f1865ff25ec02e796951c5b5a4f020/install.sh -o "$installer"
export XUI_NONINTERACTIVE=1 XUI_ENABLE_FAIL2BAN=false XUI_SSL_MODE=none
export XUI_PANEL_PORT=%d XUI_WEB_BASE_PATH=%s XUI_SERVER_IP=%s
bash "$installer" v3.8.5-farstar.1 >/var/log/farstar-node-install.log 2>&1
systemctl stop x-ui
mkdir -p /etc/x-ui/farstar-node
openssl req -x509 -newkey rsa:3072 -nodes -days 365 -subj /CN=FARSTAR-node -keyout /etc/x-ui/farstar-node/key.pem -out /etc/x-ui/farstar-node/cert.pem >/dev/null 2>&1
/usr/local/x-ui/x-ui setting -webCert /etc/x-ui/farstar-node/cert.pem -webCertKey /etc/x-ui/farstar-node/key.pem >/dev/null
systemctl start x-ui
command -v sqlite3 >/dev/null || { apt-get update >/var/log/farstar-node-packages.log 2>&1; apt-get install -y sqlite3 >>/var/log/farstar-node-packages.log 2>&1; }
`, node.Port, node.Port, shellQuote(node.BasePath), shellQuote(node.Address))
}

func sshRun(client *ssh.Client, command string, input *strings.Reader) ([]byte, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	var out boundedSSHOutput
	session.Stdout = &out
	session.Stderr = &out
	if input != nil {
		session.Stdin = input
	}
	err = session.Run(command)
	if out.exceeded {
		return nil, errors.New("SSH response exceeded output limit")
	}
	return out.buffer.Bytes(), err
}

type boundedSSHOutput struct {
	mutex    sync.Mutex
	buffer   bytes.Buffer
	exceeded bool
}

func (out *boundedSSHOutput) Write(data []byte) (int, error) {
	out.mutex.Lock()
	defer out.mutex.Unlock()
	available := (1 << 20) - out.buffer.Len()
	if len(data) > available {
		out.exceeded = true
	}
	_, _ = out.buffer.Write(data[:min(len(data), available)])
	return len(data), nil
}
