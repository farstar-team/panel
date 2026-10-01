package service

import (
	"errors"
	"net/url"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type StoreAdminState struct {
	Config model.StoreConfig `json:"config"`
	Plans  []model.StorePlan `json:"plans"`
}

func (s *StoreService) UpdateStoreSettings(cfg model.StoreConfig) error {
	cfg.PaymentText = strings.TrimSpace(cfg.PaymentText)
	cfg.SupportURL = strings.TrimSpace(cfg.SupportURL)
	if len(cfg.PaymentText) > 4000 {
		return errors.New("متن پرداخت نباید بیشتر از ۴۰۰۰ بایت باشد")
	}
	if cfg.SupportURL != "" {
		u, err := url.Parse(cfg.SupportURL)
		if err != nil || u.Scheme != "https" || u.Host != "t.me" || u.User != nil || u.Path == "" || u.Path == "/" {
			return errors.New("لینک پشتیبانی باید https://t.me/username باشد")
		}
	}
	if cfg.Enabled {
		settings := &SettingService{}
		bot, botErr := settings.GetTgbotEnabled()
		sub, subErr := settings.GetSubEnable()
		plans, planErr := s.Plans()
		if botErr != nil || subErr != nil || planErr != nil || !bot || !sub || len(plans) == 0 || cfg.PaymentText == "" {
			return errors.New("ابتدا ربات، سابسکرایب، روش پرداخت و حداقل یک پلن فعال را تنظیم کنید")
		}
	}
	return s.SaveConfig(cfg)
}

func (s *StoreService) SavePlan(plan model.StorePlan) error {
	plan.Name = strings.TrimSpace(plan.Name)
	if err := validateStorePlan(plan); err != nil {
		return err
	}
	db := database.GetDB()
	if plan.ID == 0 {
		return db.Create(&plan).Error
	}
	if plan.ID < 0 {
		return errors.New("شماره پلن معتبر نیست")
	}
	var existing model.StorePlan
	if err := db.First(&existing, plan.ID).Error; err != nil {
		return err
	}
	return db.Model(&existing).Updates(map[string]any{"name": plan.Name, "price": plan.Price, "quota_bytes": plan.QuotaBytes, "days": plan.Days, "enabled": plan.Enabled}).Error
}

func validateStorePlan(plan model.StorePlan) error {
	if strings.TrimSpace(plan.Name) == "" || len(plan.Name) > 150 || plan.Price <= 0 || plan.Price > 1_000_000_000_000 || plan.QuotaBytes < 1 || plan.QuotaBytes > 100_000*(1<<30) || plan.Days < 1 || plan.Days > 3650 {
		return errors.New("نام، قیمت، حجم یا مدت پلن معتبر نیست")
	}
	return nil
}

func (s *StoreService) AdminState() (*StoreAdminState, error) {
	state := &StoreAdminState{Config: s.Config(), Plans: []model.StorePlan{}}
	err := database.GetDB().Order("id ASC").Find(&state.Plans).Error
	return state, err
}
