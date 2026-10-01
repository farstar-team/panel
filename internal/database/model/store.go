package model

type StoreConfig struct {
	ID          int    `gorm:"primaryKey" json:"id"`
	Enabled     bool   `json:"enabled"`
	PaymentText string `json:"paymentText"`
	SupportURL  string `json:"supportURL"`
}

type StorePlan struct {
	ID         int    `gorm:"primaryKey" json:"id"`
	Name       string `json:"name"`
	Price      int64  `json:"price"`
	QuotaBytes int64  `json:"quotaBytes"`
	Days       int    `json:"days"`
	Enabled    bool   `json:"enabled"`
}

type StoreOrder struct {
	ID               int   `gorm:"primaryKey"`
	TgID             int64 `gorm:"index"`
	PlanID           int
	PlanName         string
	Price            int64
	QuotaBytes       int64
	Days             int
	Email            string
	Renew            bool
	Status           string `gorm:"index"`
	ReceiptChatID    int64
	ReceiptMessageID int
	TargetQuota      int64
	TargetExpiry     int64
	InboundIDs       string
	ApprovedBy       int64
	CreatedAt        int64 `gorm:"autoCreateTime:milli"`
	UpdatedAt        int64 `gorm:"autoUpdateTime:milli"`
}
