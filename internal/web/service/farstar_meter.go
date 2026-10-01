package service

import (
	"fmt"
	"math"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
	"gorm.io/gorm"
)

func normalizeTrafficMultiplier(inbound *model.Inbound) error {
	if inbound.TrafficMultiplier == 0 { inbound.TrafficMultiplier = 1 }
	if math.IsNaN(inbound.TrafficMultiplier) || math.IsInf(inbound.TrafficMultiplier, 0) || inbound.TrafficMultiplier < .01 || inbound.TrafficMultiplier > 100 {
		return fmt.Errorf("trafficMultiplier must be between 0.01 and 100")
	}
	return nil
}

func chargedBytes(raw int64, multiplier float64) int64 {
	if raw <= 0 { return 0 }
	if multiplier <= 0 { multiplier = 1 }
	charged := math.Round(float64(raw) * multiplier)
	if charged >= float64(database.TrafficMax) { return database.TrafficMax }
	return int64(charged)
}

func addChargedBytes(current, delta int64) int64 {
	if delta >= database.TrafficMax-current { return database.TrafficMax }
	return current + delta
}

func meteredClientTraffic(tx *gorm.DB, rows []*xray.ClientTraffic) ([]*xray.ClientTraffic, error) {
	ids := make([]int, 0)
	for _, row := range rows { if row != nil { id, _ := xray.ParseMeterEmail(row.Email); if id == 0 { id = row.InboundId }; if id > 0 { ids = append(ids, id) } } }
	inbounds := make([]model.Inbound, 0)
	if len(ids) > 0 { if err := tx.Where("id IN ? AND node_id IS NULL", ids).Find(&inbounds).Error; err != nil { return nil, err } }
	rates := make(map[int]float64, len(inbounds))
	for _, inbound := range inbounds { rates[inbound.Id] = inbound.TrafficMultiplier }
	byEmail := make(map[string]*xray.ClientTraffic)
	for _, row := range rows {
		if row == nil { continue }
		id, email := xray.ParseMeterEmail(row.Email)
		if id == 0 { id = row.InboundId }
		rate := rates[id]
		if rate <= 0 { rate = 1 }
		merged := byEmail[email]
		if merged == nil { merged = &xray.ClientTraffic{Email: email}; byEmail[email] = merged }
		merged.Up = addChargedBytes(merged.Up, chargedBytes(row.Up, rate))
		merged.Down = addChargedBytes(merged.Down, chargedBytes(row.Down, rate))
	}
	result := make([]*xray.ClientTraffic, 0, len(byEmail))
	for _, row := range byEmail { result = append(result, row) }
	return result, nil
}
