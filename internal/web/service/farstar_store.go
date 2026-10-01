package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

var storeMutex sync.Mutex

type StoreService struct{}

func (s *StoreService) Config() model.StoreConfig {
	var cfg model.StoreConfig
	database.GetDB().First(&cfg, 1)
	return cfg
}

func (s *StoreService) SaveConfig(cfg model.StoreConfig) error {
	cfg.ID = 1
	return database.GetDB().Save(&cfg).Error
}

func (s *StoreService) Plans() ([]model.StorePlan, error) {
	var plans []model.StorePlan
	err := database.GetDB().Where("enabled = ?", true).Order("price ASC").Find(&plans).Error
	return plans, err
}

func (s *StoreService) AddPlan(plan model.StorePlan) error {
	if err := validateStorePlan(plan); err != nil {
		return err
	}
	plan.Enabled = true
	return database.GetDB().Create(&plan).Error
}

func (s *StoreService) NewOrder(tgID int64, planID int, renewEmail string) (*model.StoreOrder, error) {
	storeMutex.Lock()
	defer storeMutex.Unlock()
	if tgID <= 0 || !s.Config().Enabled {
		return nil, errors.New("فروشگاه فعال نیست")
	}
	var order model.StoreOrder
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var plan model.StorePlan
		if err := tx.Where("id = ? AND enabled = ?", planID, true).First(&plan).Error; err != nil {
			return errors.New("پلن موجود نیست")
		}
		tx.Model(&model.StoreOrder{}).Where("tg_id = ? AND status = ? AND created_at < ?", tgID, "pending", time.Now().Add(-24*time.Hour).UnixMilli()).Update("status", "cancelled")
		var count int64
		if err := tx.Model(&model.StoreOrder{}).Where("tg_id = ? AND status IN ?", tgID, []string{"pending", "review", "fulfilling"}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("یک سفارش باز دارید؛ ابتدا آن را تکمیل یا لغو کنید")
		}
		if renewEmail != "" {
			var rec model.ClientRecord
			if err := tx.Where("email = ? AND tg_id = ?", renewEmail, tgID).First(&rec).Error; err != nil {
				return errors.New("سرویس متعلق به شما نیست")
			}
			if rec.TotalGB == 0 {
				return errors.New("سرویس نامحدود نیاز به تمدید حجمی ندارد")
			}
		}
		order = model.StoreOrder{TgID: tgID, PlanID: plan.ID, PlanName: plan.Name, Price: plan.Price, QuotaBytes: plan.QuotaBytes, Days: plan.Days, Email: renewEmail, Renew: renewEmail != "", Status: "pending"}
		return tx.Create(&order).Error
	})
	return &order, err
}

func (s *StoreService) Receipt(tgID, chatID int64, messageID int) (*model.StoreOrder, error) {
	storeMutex.Lock()
	defer storeMutex.Unlock()
	var order model.StoreOrder
	db := database.GetDB()
	if err := db.Where("tg_id = ? AND status = ?", tgID, "pending").Order("id DESC").First(&order).Error; err != nil {
		return nil, errors.New("سفارش پرداخت\u200cنشده\u200cای ندارید")
	}
	if order.CreatedAt < time.Now().Add(-24*time.Hour).UnixMilli() {
		return nil, errors.New("مهلت سفارش تمام شده؛ دوباره سفارش بدهید")
	}
	if err := db.Model(&order).Updates(map[string]any{"status": "review", "receipt_chat_id": chatID, "receipt_message_id": messageID}).Error; err != nil {
		return nil, err
	}
	order.Status = "review"
	return &order, nil
}

func (s *StoreService) Cancel(tgID int64, orderID int) error {
	storeMutex.Lock()
	defer storeMutex.Unlock()
	result := database.GetDB().Model(&model.StoreOrder{}).Where("id = ? AND tg_id = ? AND status = ?", orderID, tgID, "pending").Update("status", "cancelled")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("سفارش قابل لغو نیست")
	}
	return nil
}

func (s *StoreService) Orders(tgID int64) ([]model.StoreOrder, error) {
	var orders []model.StoreOrder
	query := database.GetDB().Order("id DESC").Limit(20)
	if tgID > 0 {
		query = query.Where("tg_id = ?", tgID)
	} else {
		query = query.Where("status IN ?", []string{"review", "fulfilling"})
	}
	err := query.Find(&orders).Error
	return orders, err
}

func (s *StoreService) Reject(orderID int, adminID int64) (*model.StoreOrder, error) {
	storeMutex.Lock()
	defer storeMutex.Unlock()
	var order model.StoreOrder
	if err := database.GetDB().First(&order, orderID).Error; err != nil {
		return nil, err
	}
	if order.Status != "review" {
		return nil, errors.New("سفارش در انتظار بررسی نیست")
	}
	return &order, database.GetDB().Model(&order).Updates(map[string]any{"status": "rejected", "approved_by": adminID}).Error
}

func (s *StoreService) Approve(orderID int, adminID int64, inboundSvc *InboundService) (*model.StoreOrder, bool, error) {
	storeMutex.Lock()
	defer storeMutex.Unlock()
	db := database.GetDB()
	var order model.StoreOrder
	if err := db.First(&order, orderID).Error; err != nil {
		return nil, false, err
	}
	if order.Status == "fulfilled" {
		return &order, false, nil
	}
	if order.Status != "review" && order.Status != "fulfilling" {
		return nil, false, errors.New("سفارش قابل تأیید نیست")
	}
	clientSvc := &ClientService{}
	if order.TargetExpiry == 0 {
		order.TargetQuota = order.QuotaBytes
		order.TargetExpiry = time.Now().Add(time.Duration(order.Days) * 24 * time.Hour).UnixMilli()
		if order.Renew {
			rec, err := clientSvc.GetRecordByEmail(nil, order.Email)
			if err != nil || rec.TgID != order.TgID {
				return nil, false, errors.New("مالکیت سرویس تغییر کرده است")
			}
			order.TargetQuota = addChargedBytes(rec.TotalGB, order.QuotaBytes)
			order.TargetExpiry = max(time.Now().UnixMilli(), rec.ExpiryTime) + int64(order.Days)*86400000
		} else {
			order.Email = fmt.Sprintf("shop-%d-%d", order.TgID, order.ID)
			inbounds, listErr := inboundSvc.GetAllInbounds()
			if listErr != nil {
				return nil, false, listErr
			}
			ids := []int{}
			for _, inbound := range inbounds {
				if inbound.Enable && !inbound.ExcludeFromSub {
					switch inbound.Protocol {
					case model.VLESS, model.VMESS, model.Trojan, model.Shadowsocks, model.Hysteria:
						ids = append(ids, inbound.Id)
					}
				}
			}
			if len(ids) == 0 {
				return nil, false, errors.New("اینباند فعال و مناسب فروش ندارید")
			}
			encoded, err := json.Marshal(ids)
			if err != nil {
				return nil, false, err
			}
			order.InboundIDs = string(encoded)
		}
		order.Status = "fulfilling"
		order.ApprovedBy = adminID
		if err := db.Save(&order).Error; err != nil {
			return nil, false, err
		}
	}
	var restart bool
	var err error
	if order.Renew {
		rec, getErr := clientSvc.GetRecordByEmail(nil, order.Email)
		if getErr != nil || rec.TgID != order.TgID {
			return nil, false, errors.New("سرویس تمدیدی یافت نشد")
		}
		_, client, getErr := inboundSvc.GetClientByEmail(order.Email)
		if getErr != nil || client.TgID != order.TgID {
			return nil, false, errors.New("سرویس تمدیدی یافت نشد")
		}
		client.TotalGB, client.ExpiryTime, client.Enable = order.TargetQuota, order.TargetExpiry, true
		restart, err = clientSvc.UpdateByEmail(inboundSvc, order.Email, *client, rec.LimitHwid)
	} else {
		var ids []int
		if err := json.Unmarshal([]byte(order.InboundIDs), &ids); err != nil {
			return nil, false, err
		}
		rec, getErr := clientSvc.GetRecordByEmail(nil, order.Email)
		if errors.Is(getErr, gorm.ErrRecordNotFound) {
			client := model.Client{Email: order.Email, ID: uuid.NewString(), SubID: strings.ReplaceAll(uuid.NewString(), "-", ""), TgID: order.TgID, TotalGB: order.TargetQuota, ExpiryTime: order.TargetExpiry, Enable: true, Comment: fmt.Sprintf("FARSTAR order #%d", order.ID)}
			restart, err = clientSvc.Create(inboundSvc, &ClientCreatePayload{Client: client, InboundIds: ids})
		} else if getErr != nil {
			err = getErr
		} else if rec.TgID != order.TgID {
			err = errors.New("هویت سفارش ناسازگار است")
		} else {
			restart, err = clientSvc.Attach(inboundSvc, rec.Id, ids)
		}
	}
	if err != nil {
		return &order, restart, err
	}
	order.Status = "fulfilled"
	err = db.Model(&order).Update("status", "fulfilled").Error
	return &order, restart, err
}
