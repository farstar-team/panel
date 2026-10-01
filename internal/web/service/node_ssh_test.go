package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/crypto/nodetoken"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestNodeSSHSecretsAreEncryptedAndWriteOnly(t *testing.T) {
	setupConflictDB(t)
	enableNodeTokenEncryption(t)
	token := "node-token"
	password := "ssh-secret"
	request := &NodeMutationRequest{Name: "iran-edge", Address: "203.0.113.20", Port: 2053, ApiToken: &token, Region: "iran", SSH: &NodeSSHRequest{Port: 22, Username: "root", Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Password: &password, TrustConfirmed: true}}
	view, err := (&NodeService{}).CreateFromRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	var stored model.Node
	if err := database.GetDB().First(&stored, view.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !nodetoken.IsEncrypted(stored.SSHPassword) {
		t.Fatal("SSH password was stored as plaintext")
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), password) || strings.Contains(string(raw), "enc:v1:") {
		t.Fatal("SSH secret leaked in node response")
	}
	if view.Region != "iran" || !view.HasSSHCredentials {
		t.Fatalf("node metadata = %+v", view)
	}
	if err := database.GetDB().Create(&model.Node{Name: "foreign", Address: "203.0.113.21", Port: 2053, ApiToken: ""}).Error; err != nil {
		t.Fatal(err)
	}
	var original model.Node
	if err := database.GetDB().First(&original, view.Id).Error; err != nil {
		t.Fatal(err)
	}
	decrypted, err := nodetoken.DecryptBound(sshBinding(view.Id, "password"), original.SSHPassword)
	if err != nil || decrypted != password {
		t.Fatalf("saved secret did not round-trip: %v", err)
	}
	if _, err := nodetoken.DecryptBound([]byte("nodes/ssh/2/password"), original.SSHPassword); err == nil || !strings.Contains(err.Error(), "nodetoken: authentication failed:") {
		t.Fatalf("cross-node secret binding failed: %v", err)
	}
}

func TestNodeSSHRefusesPlaintextStorageWhenEncryptionIsOff(t *testing.T) {
	setupConflictDB(t)
	token, password := "node-token", "ssh-secret"
	_, err := (&NodeService{}).CreateFromRequest(&NodeMutationRequest{Name: "iran", Address: "203.0.113.20", Port: 2053, ApiToken: &token, SSH: &NodeSSHRequest{Port: 22, Username: "root", Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Password: &password}})
	if err == nil || err.Error() != "SSH credentials require NODE_TOKEN_ENCRYPTION=required and a protected key file" {
		t.Fatalf("unencrypted SSH save = %v", err)
	}
	var count int64
	database.GetDB().Model(&model.Node{}).Count(&count)
	if count != 0 {
		t.Fatalf("rejected request left %d nodes", count)
	}
}

func TestNodeSSHAddressChangeRequiresNewTrustConfirmation(t *testing.T) {
	setupConflictDB(t)
	enableNodeTokenEncryption(t)
	token, password := "node-token", "ssh-private"
	service := &NodeService{}
	view, err := service.CreateFromRequest(&NodeMutationRequest{Name: "iran", Address: "203.0.113.20", Port: 2053, ApiToken: &token, Region: "iran", SSH: &NodeSSHRequest{Port: 22, Username: "root", Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Password: &password, TrustConfirmed: true}})
	if err != nil {
		t.Fatal(err)
	}
	unchanged := &NodeMutationRequest{Name: "renamed", Address: "203.0.113.20", Port: 2053}
	if err := service.UpdateFromRequest(view.Id, unchanged); err != nil {
		t.Fatal(err)
	}
	var before model.Node
	if err := database.GetDB().First(&before, view.Id).Error; err != nil {
		t.Fatal(err)
	}
	if before.Region != "iran" {
		t.Fatalf("region lost after legacy edit: %q", before.Region)
	}
	unchanged.Address = "203.0.113.21"
	err = service.UpdateFromRequest(view.Id, unchanged)
	if err == nil || err.Error() != "confirm SSH trust again before changing the server address" {
		t.Fatalf("address change reused SSH credentials: %v", err)
	}
	var after model.Node
	if err := database.GetDB().First(&after, view.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.Address != before.Address || after.SSHPassword != before.SSHPassword {
		t.Fatal("rejected address change modified the stored server or secret")
	}
}

func TestNodeCreateKeepsProvisioningNodeDisabled(t *testing.T) {
	setupConflictDB(t)
	enableNodeTokenEncryption(t)
	node := &model.Node{Name: "installing", Address: "203.0.113.20", Port: 2053, Enable: false, ProvisionStatus: "pending"}
	if err := (&NodeService{}).Create(node); err != nil {
		t.Fatal(err)
	}
	var stored model.Node
	if err := database.GetDB().First(&stored, node.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Enable {
		t.Fatal("provisioning node became enabled before a successful pinned HTTPS probe")
	}
}

func TestNodeProvisioningCannotBeChangedOrDeleted(t *testing.T) {
	for _, status := range []string{"pending", "running"} {
		for _, action := range []string{"edit", "legacy-edit", "enable", "delete"} {
			t.Run(status+"/"+action, func(t *testing.T) {
				setupConflictDB(t)
				enableNodeTokenEncryption(t)
				node := &model.Node{Name: "installing", Address: "203.0.113.20", Port: 2053, Enable: false, ProvisionStatus: status}
				service := &NodeService{}
				if err := service.Create(node); err != nil {
					t.Fatal(err)
				}
				var err error
				switch action {
				case "edit":
					err = service.UpdateFromRequest(node.Id, &NodeMutationRequest{Name: "changed", Address: "203.0.113.21", Port: 2053})
				case "legacy-edit":
					err = service.Update(node.Id, &model.Node{Name: "changed", Address: "203.0.113.21", Port: 2053})
				case "enable":
					err = service.SetEnable(node.Id, true)
				case "delete":
					err = service.Delete(node.Id)
				}
				if err == nil || err.Error() != "automatic installation is running; wait for it to finish before changing or deleting this node" {
					t.Fatalf("%s during %s: %v", action, status, err)
				}
				var stored model.Node
				if err := database.GetDB().First(&stored, node.Id).Error; err != nil {
					t.Fatal(err)
				}
				if stored.Name != node.Name || stored.Address != node.Address || stored.Enable || stored.ProvisionStatus != status {
					t.Fatal("installation state changed during the rejected operation")
				}
			})
		}
	}
}
