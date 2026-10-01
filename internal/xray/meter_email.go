package xray

import (
	"encoding/base64"
	"strconv"
	"strings"
)

func MeterEmail(inboundID int, email string) string {
	if inboundID <= 0 || email == "" {
		return email
	}
	return "fs." + strconv.Itoa(inboundID) + "." + base64.RawURLEncoding.EncodeToString([]byte(email))
}

func ParseMeterEmail(email string) (int, string) {
	parts := strings.SplitN(email, ".", 3)
	if len(parts) != 3 || parts[0] != "fs" {
		return 0, email
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil || id <= 0 {
		return 0, email
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(decoded) == 0 {
		return 0, email
	}
	return id, string(decoded)
}
