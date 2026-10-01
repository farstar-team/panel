package service

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func TestFarstarMeterChargesOnlyTheInboundThatTransferredBytes(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	db := database.GetDB()
	if !db.Migrator().HasColumn(&model.Inbound{}, "traffic_multiplier") {
		if err := db.Exec("ALTER TABLE inbounds ADD COLUMN traffic_multiplier real DEFAULT 1").Error; err != nil {
			t.Fatal(err)
		}
	}
	ordinary := model.Inbound{Tag: "ordinary", Protocol: model.VLESS, Enable: true, Port: 22001, Settings: `{"clients":[]}`}
	premium := model.Inbound{Tag: "premium", Protocol: model.VLESS, Enable: true, Port: 22002, Settings: `{"clients":[]}`}
	for _, inbound := range []*model.Inbound{&ordinary, &premium} {
		if err := db.Create(inbound).Error; err != nil { t.Fatal(err) }
	}
	if err := db.Model(&premium).UpdateColumn("traffic_multiplier", 2).Error; err != nil { t.Fatal(err) }
	if err := db.Create(&xray.ClientTraffic{Email: "shared@x", InboundId: ordinary.Id, Enable: true, Total: 10000}).Error; err != nil { t.Fatal(err) }
	alias := func(id int) string { return fmt.Sprintf("fs.%d.%s", id, base64.RawURLEncoding.EncodeToString([]byte("shared@x"))) }
	svc := &InboundService{}
	if _, _, err := svc.AddTraffic(nil, []*xray.ClientTraffic{
		{Email: alias(ordinary.Id), Up: 100, Down: 200},
		{Email: alias(premium.Id), Up: 100, Down: 200},
	}); err != nil { t.Fatal(err) }
	var got xray.ClientTraffic
	if err := db.Where("email = ?", "shared@x").First(&got).Error; err != nil { t.Fatal(err) }
	if got.Up != 300 || got.Down != 600 { t.Fatalf("charged up/down = %d/%d; want 300/600", got.Up, got.Down) }
	if err := db.Model(&premium).UpdateColumn("traffic_multiplier", 3).Error; err != nil { t.Fatal(err) }
	if _, _, err := svc.AddTraffic(nil, []*xray.ClientTraffic{{Email: alias(premium.Id), Down: 100}}); err != nil { t.Fatal(err) }
	if err := db.Where("email = ?", "shared@x").First(&got).Error; err != nil { t.Fatal(err) }
	if got.Down != 900 { t.Fatalf("multiplier edit rewrote history: down=%d; want 900", got.Down) }
}
