package service

import (
	"encoding/json"

	"github.com/mhsanaei/3x-ui/v3/internal/util/json_util"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func expandMeteredRoutingUsers(cfg *xray.Config) {
	aliases := map[string][]string{}
	for _, inbound := range cfg.InboundConfigs {
		var settings map[string]any
		if json.Unmarshal(inbound.Settings, &settings) != nil {
			continue
		}
		for _, key := range []string{"clients", "accounts", "peers"} {
			entries, _ := settings[key].([]any)
			for _, raw := range entries {
				entry, _ := raw.(map[string]any)
				email, _ := entry["email"].(string)
				if id, original := xray.ParseMeterEmail(email); id > 0 {
					aliases[original] = append(aliases[original], email)
				}
			}
		}
	}
	var routing map[string]any
	if json.Unmarshal(cfg.RouterConfig, &routing) != nil {
		return
	}
	rules, _ := routing["rules"].([]any)
	for _, raw := range rules {
		rule, _ := raw.(map[string]any)
		users, ok := rule["user"].([]any)
		if !ok {
			continue
		}
		for _, user := range users {
			if email, ok := user.(string); ok {
				for _, alias := range aliases[email] {
					users = append(users, alias)
				}
			}
		}
		rule["user"] = users
	}
	if encoded, err := json.Marshal(routing); err == nil {
		cfg.RouterConfig = json_util.RawMessage(encoded)
	}
}
