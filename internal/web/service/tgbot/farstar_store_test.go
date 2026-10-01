package tgbot

import (
	"fmt"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mymmrac/telego"
)

func TestFarstarStoreRejectsForgedApprovalAndForeignCancellation(t *testing.T) {
	tb, _ := newLevelTgbot(t)
	s := &service.StoreService{}
	if err := s.SaveConfig(model.StoreConfig{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "reject", "cancel"} {
		status := "review"
		if action == "cancel" {
			status = "pending"
		}
		order := model.StoreOrder{TgID: ownerTgID, Status: status, PlanName: "plan", Price: 100, Renew: true, Email: ownerMail, Days: 30, QuotaBytes: 1 << 30}
		if err := database.GetDB().Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		q := &telego.CallbackQuery{ID: "store-test", From: telego.User{ID: 777}, Data: fmt.Sprintf("shop:%s:%d", action, order.ID), Message: &telego.Message{Chat: telego.Chat{ID: 777}}}
		if !tb.handleStoreCallback(q) {
			t.Fatal("store callback escaped its dispatcher")
		}
		var after model.StoreOrder
		if err := database.GetDB().First(&after, order.ID).Error; err != nil {
			t.Fatal(err)
		}
		if after.Status != status || after.ApprovedBy != 0 {
			t.Fatalf("forged %s changed order: %+v", action, after)
		}
	}
}
