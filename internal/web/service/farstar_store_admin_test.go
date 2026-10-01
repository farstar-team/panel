package service

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestStoreAdminCannotOpenSalesWithoutPaymentOrPlans(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	svc := &StoreService{}
	if err := svc.UpdateStoreSettings(model.StoreConfig{Enabled: true}); err == nil || err.Error() != "ابتدا ربات، سابسکرایب، روش پرداخت و حداقل یک پلن فعال را تنظیم کنید" {
		t.Fatalf("open unconfigured store = %v", err)
	}
	if svc.Config().Enabled {
		t.Fatal("failed validation enabled the store")
	}
}

func TestStoreAdminEditsPlanWithoutChangingExistingOrder(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	svc := &StoreService{}
	if err := svc.AddPlan(model.StorePlan{Name: "old", Price: 100000, QuotaBytes: 10 << 30, Days: 30}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveConfig(model.StoreConfig{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	order, err := svc.NewOrder(123, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SavePlan(model.StorePlan{ID: 1, Name: "new", Price: 200000, QuotaBytes: 20 << 30, Days: 60, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	var saved model.StoreOrder
	if err := database.GetDB().First(&saved, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Price != 100000 || saved.QuotaBytes != 10<<30 || saved.Days != 30 {
		t.Fatalf("existing order changed: %+v", saved)
	}
	plans, err := svc.Plans()
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 0 {
		t.Fatalf("disabled plan is still on sale: %+v", plans)
	}
	state, err := svc.AdminState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Plans) != 1 || state.Plans[0].Name != "new" || state.Plans[0].Enabled {
		t.Fatalf("admin cannot recover disabled plan: %+v", state.Plans)
	}
}
