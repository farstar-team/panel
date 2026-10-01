package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"path/filepath"
	"testing"
)

func TestFarstarStoreSnapshotsPriceAndProtectsOwnership(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	s := &StoreService{}
	if err := s.SaveConfig(model.StoreConfig{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddPlan(model.StorePlan{Name: "یک ماه", Price: 100000, QuotaBytes: 10 << 30, Days: 30}); err != nil {
		t.Fatal(err)
	}
	order, err := s.NewOrder(123, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	database.GetDB().Model(&model.StorePlan{}).Where("id = ?", 1).Update("price", 200000)
	if order.Price != 100000 {
		t.Fatalf("price not snapshotted: %d", order.Price)
	}
	if _, err := s.NewOrder(123, 1, ""); err == nil {
		t.Fatal("duplicate pending order accepted")
	}
	if err := s.Cancel(456, order.ID); err == nil {
		t.Fatal("other user cancelled order")
	}
	if _, err := s.Receipt(456, 456, 1); err == nil {
		t.Fatal("other user's receipt accepted")
	}
	if _, _, err := s.Approve(order.ID, 1, &InboundService{}); err == nil {
		t.Fatal("unpaid order approved")
	}
	if _, err := s.Receipt(123, 123, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel(123, order.ID); err == nil {
		t.Fatal("review order cancelled")
	}
	if _, err := s.Reject(order.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Approve(order.ID, 1, &InboundService{}); err == nil {
		t.Fatal("rejected order approved")
	}
}

func TestFarstarStoreDeliveryAndRenewalAreIdempotent(t *testing.T) {
	setupBulkDB(t)
	inboundSvc := &InboundService{}
	mkInbound(t, 24901, model.VLESS, `{"clients":[]}`)
	mkInbound(t, 24902, model.VMESS, `{"clients":[]}`)
	s := &StoreService{}
	if err := s.SaveConfig(model.StoreConfig{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddPlan(model.StorePlan{Name: "test", Price: 100, QuotaBytes: 10 << 30, Days: 30}); err != nil {
		t.Fatal(err)
	}
	order, err := s.NewOrder(123, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Receipt(123, 123, 1); err != nil {
		t.Fatal(err)
	}
	order, _, err = s.Approve(order.ID, 1, inboundSvc)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Approve(order.ID, 1, inboundSvc); err != nil {
		t.Fatal(err)
	}
	rec := lookupClientRecord(t, order.Email)
	ids, err := (&ClientService{}).GetInboundIdsForRecord(rec.Id)
	if err != nil || len(ids) != 2 {
		t.Fatalf("attachments: %v, %v", ids, err)
	}
	if n := countClientRecords(t); n != 1 {
		t.Fatalf("duplicate delivery: %d clients", n)
	}
	if _, err := s.NewOrder(456, 1, order.Email); err == nil {
		t.Fatal("other user's renewal accepted")
	}
	renew, err := s.NewOrder(123, 1, order.Email)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Receipt(123, 123, 2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Approve(renew.ID, 1, inboundSvc); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Approve(renew.ID, 1, inboundSvc); err != nil {
		t.Fatal(err)
	}
	after := lookupClientRecord(t, order.Email)
	if after.TotalGB != 20<<30 || after.ExpiryTime != rec.ExpiryTime+30*86400000 || after.SubID != rec.SubID {
		t.Fatalf("renewal changed more than once: %+v", after)
	}
}
