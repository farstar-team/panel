package model

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func MeterInboundSettings(id int, settings string) string {
	if id <= 0 { return settings }
	var parsed map[string]any
	if json.Unmarshal([]byte(settings), &parsed) != nil { return settings }
	for _, key := range []string{"clients", "peers", "accounts"} {
		entries, _ := parsed[key].([]any)
		for _, raw := range entries {
			entry, ok := raw.(map[string]any)
			if !ok { continue }
			if email, ok := entry["email"].(string); ok {
				entry["email"] = xray.MeterEmail(id, email)
			}
		}
	}
	result, err := json.Marshal(parsed)
	if err != nil { return settings }
	return string(result)
}
