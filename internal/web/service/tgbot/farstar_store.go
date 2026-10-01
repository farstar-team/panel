package tgbot

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/skip2/go-qrcode"
)

const storeHelp = "<b>فروشگاه FARSTAR</b>\n/shop فروشگاه\n/my سرویس‌ها و سفارش‌های من\n\n<b>دستورهای مدیر</b>\n/plan نام_پلن قیمت_تومان حجم_GB روز\n/plan_off شماره\n/payment متن روش پرداخت\n/support https://t.me/username\n/sales on یا off\n/orders سفارش‌های منتظر بررسی\n/approve شماره\n/reject شماره\n\nفقط پس از بررسی واقعی واریز، پرداخت را تأیید کنید."

func (t *Tgbot) handleStoreCommand(message *telego.Message, command string, args []string, isAdmin bool) bool {
	s := &service.StoreService{}
	chatID := message.Chat.ID
	if command == "start" && !isAdmin && len(args) == 0 && s.Config().Enabled && chatID == message.From.ID { t.storePlans(chatID, 0); return true }
	switch command { case "shop", "my", "sales", "plan", "plan_off", "payment", "support", "orders", "approve", "reject", "storehelp": default: return false }
	if chatID != message.From.ID { t.SendMsgToTgbot(chatID, "فروشگاه فقط در گفت‌وگوی خصوصی فعال است."); return true }
	if command == "shop" { if s.Config().Enabled { t.storePlans(chatID, 0) }; return true }
	if command == "my" { t.storeAccount(chatID); return true }
	if !isAdmin { return true }
	var err error
	cfg := s.Config()
	switch command {
	case "storehelp": t.SendMsgToTgbot(chatID, storeHelp); return true
	case "sales":
		if len(args) != 1 || (args[0] != "on" && args[0] != "off") { t.SendMsgToTgbot(chatID, storeHelp); return true }
		if args[0] == "on" {
			enabled, subErr := t.settingService.GetSubEnable(); plans, planErr := s.Plans()
			if subErr != nil || !enabled || planErr != nil || len(plans) == 0 || cfg.PaymentText == "" { t.SendMsgToTgbot(chatID, "ابتدا سابسکرایب پنل، یک پلن و متن پرداخت را تنظیم کنید."); return true }
		}
		cfg.Enabled = args[0] == "on"; err = s.SaveConfig(cfg)
	case "payment": cfg.PaymentText = strings.Join(args, " "); err = s.SaveConfig(cfg)
	case "support":
		if len(args) != 1 { t.SendMsgToTgbot(chatID, storeHelp); return true }
		u, e := url.Parse(args[0]); if e != nil || u.Scheme != "https" || u.Host != "t.me" || u.User != nil { t.SendMsgToTgbot(chatID, "لینک پشتیبانی باید https://t.me/username باشد."); return true }
		cfg.SupportURL = args[0]; err = s.SaveConfig(cfg)
	case "plan":
		if len(args) != 4 { t.SendMsgToTgbot(chatID, storeHelp); return true }
		price, e1 := strconv.ParseInt(args[1], 10, 64); gb, e2 := strconv.ParseInt(args[2], 10, 64); days, e3 := strconv.Atoi(args[3])
		if e1 != nil || e2 != nil || e3 != nil || gb < 1 || gb > 100000 { t.SendMsgToTgbot(chatID, "قیمت، حجم و روز باید عدد صحیح مثبت باشند."); return true }
		err = s.AddPlan(model.StorePlan{Name:strings.ReplaceAll(args[0], "_", " "), Price:price, QuotaBytes:gb*(1<<30), Days:days})
	case "plan_off":
		if len(args) != 1 { t.SendMsgToTgbot(chatID, storeHelp); return true }; id, e := strconv.Atoi(args[0]); if e != nil { return true }
		err = database.GetDB().Model(&model.StorePlan{}).Where("id = ?", id).Update("enabled", false).Error
	case "orders": t.storeOrders(chatID, 0); return true
	case "approve", "reject":
		if len(args) != 1 { t.SendMsgToTgbot(chatID, storeHelp); return true }; id, e := strconv.Atoi(args[0]); if e != nil { return true }
		t.storeDecision(chatID, id, command == "approve"); return true
	}
	if err != nil { t.SendMsgToTgbot(chatID, html.EscapeString(err.Error())) } else { t.SendMsgToTgbot(chatID, "ذخیره شد. /storehelp") }
	return true
}

func (t *Tgbot) storePlans(chatID int64, renewClientID int) {
	s := &service.StoreService{}; plans, err := s.Plans()
	if err != nil { t.SendMsgToTgbot(chatID, "دریافت پلن‌ها ناموفق بود."); return }
	rows := [][]telego.InlineKeyboardButton{}
	for _, plan := range plans {
		label := fmt.Sprintf("%s • %d GB • %d روز • %d تومان", plan.Name, plan.QuotaBytes/(1<<30), plan.Days, plan.Price)
		rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(label).WithCallbackData(fmt.Sprintf("shop:buy:%d:%d", plan.ID, renewClientID))))
	}
	rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton("سرویس‌ها و سفارش‌های من").WithCallbackData("shop:my")))
	if support := s.Config().SupportURL; support != "" { rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton("پشتیبانی").WithURL(support))) }
	t.SendMsgToTgbot(chatID, "<b>FARSTAR · اتصال ساده</b>\nپلن خود را انتخاب کنید. پس از تأیید رسید، لینک اشتراک و QR تحویل می‌گیرید.\nمصرف اینباندهای ویژه بر اساس ضریب اعلام‌شده محاسبه می‌شود.", tu.InlineKeyboard(rows...))
}

func (t *Tgbot) storeAccount(chatID int64) {
	records, err := t.clientService.GetRecordsByTgID(chatID)
	if err != nil { t.SendMsgToTgbot(chatID, "دریافت سرویس‌ها ناموفق بود."); return }
	for _, rec := range records {
		buttons := tu.InlineKeyboard(tu.InlineKeyboardRow(tu.InlineKeyboardButton("لینک و QR").WithCallbackData(fmt.Sprintf("shop:links:%d", rec.Id)), tu.InlineKeyboardButton("تمدید").WithCallbackData(fmt.Sprintf("shop:renew:%d", rec.Id))))
		t.SendMsgToTgbot(chatID, fmt.Sprintf("<b>%s</b>\nحجم کل: %.1f GB\nتاریخ انقضا: %s", html.EscapeString(rec.Email), float64(rec.TotalGB)/(1<<30), time.UnixMilli(rec.ExpiryTime).Format("2006-01-02")), buttons)
	}
	t.storeOrders(chatID, chatID)
}

func (t *Tgbot) storeOrders(chatID, owner int64) {
	orders, err := (&service.StoreService{}).Orders(owner)
	if err != nil { t.SendMsgToTgbot(chatID, "دریافت سفارش‌ها ناموفق بود."); return }
	if len(orders) == 0 { t.SendMsgToTgbot(chatID, "سفارشی پیدا نشد."); return }
	for _, order := range orders {
		msg := fmt.Sprintf("سفارش #%d · %s\n%d تومان · %s", order.ID, html.EscapeString(order.PlanName), order.Price, html.EscapeString(order.Status))
		if owner == 0 { t.SendMsgToTgbot(chatID, msg, storeReviewKeyboard(order.ID)) } else if order.Status == "pending" {
			t.SendMsgToTgbot(chatID, msg+"\n"+html.EscapeString((&service.StoreService{}).Config().PaymentText)+"\nعکس یا فایل رسید را ارسال کنید. ارسال رسید به معنی تأیید پرداخت نیست.", tu.InlineKeyboard(tu.InlineKeyboardRow(tu.InlineKeyboardButton("لغو سفارش").WithCallbackData(fmt.Sprintf("shop:cancel:%d", order.ID)))))
		} else { t.SendMsgToTgbot(chatID, msg) }
	}
}

func storeReviewKeyboard(id int) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(tu.InlineKeyboardRow(tu.InlineKeyboardButton("تأیید پرداخت و تحویل").WithCallbackData(fmt.Sprintf("shop:approve:%d", id)), tu.InlineKeyboardButton("رد رسید").WithCallbackData(fmt.Sprintf("shop:reject:%d", id))))
}

func (t *Tgbot) handleStoreCallback(query *telego.CallbackQuery) bool {
	if !strings.HasPrefix(query.Data, "shop:") { return false }
	actor := query.From.ID; chatID := query.Message.GetChat().ID
	if actor != chatID { t.sendCallbackAnswerTgBot(query.ID, "فقط گفت‌وگوی خصوصی"); return true }
	parts := strings.Split(query.Data, ":"); if len(parts) < 2 { return true }
	t.sendCallbackAnswerTgBot(query.ID, "")
	s := &service.StoreService{}; action := parts[1]; id := 0
	if len(parts) > 2 { id, _ = strconv.Atoi(parts[2]) }
	if action == "approve" || action == "reject" { if checkAdmin(actor) { t.storeDecision(actor, id, action == "approve") }; return true }
	if !s.Config().Enabled { t.SendMsgToTgbot(chatID, "فروشگاه غیرفعال است."); return true }
	switch action {
	case "my": t.storeAccount(chatID)
	case "cancel": if err := s.Cancel(actor, id); err != nil { t.SendMsgToTgbot(chatID, html.EscapeString(err.Error())) } else { t.SendMsgToTgbot(chatID, "سفارش لغو شد.") }
	case "links", "renew":
		var rec model.ClientRecord
		if database.GetDB().Where("id = ? AND tg_id = ?", id, actor).First(&rec).Error != nil { return true }
		if action == "links" { t.storeDeliver(chatID, rec.Email) } else { t.storePlans(chatID, rec.Id) }
	case "buy":
		renewEmail := ""
		if len(parts) > 3 && parts[3] != "0" {
			recID, err := strconv.Atoi(parts[3]); if err != nil { return true }; var rec model.ClientRecord
			if database.GetDB().Where("id = ? AND tg_id = ?", recID, actor).First(&rec).Error != nil { return true }; renewEmail = rec.Email
		}
		if _, err := s.NewOrder(actor, id, renewEmail); err != nil { t.SendMsgToTgbot(chatID, html.EscapeString(err.Error())) } else { t.storeOrders(chatID, actor) }
	}
	return true
}

func (t *Tgbot) handleStoreReceipt(message *telego.Message) bool {
	if message.From == nil || message.From.ID != message.Chat.ID || checkAdmin(message.From.ID) || (len(message.Photo) == 0 && message.Document == nil) { return false }
	s := &service.StoreService{}; if !s.Config().Enabled { return false }
	order, err := s.Receipt(message.From.ID, message.Chat.ID, message.MessageID)
	if err != nil { t.SendMsgToTgbot(message.Chat.ID, html.EscapeString(err.Error())); return true }
	for _, admin := range adminSnapshot() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		_, _ = bot.CopyMessage(ctx, &telego.CopyMessageParams{ChatID:tu.ID(admin), FromChatID:tu.ID(message.Chat.ID), MessageID:message.MessageID}); cancel()
		t.SendMsgToTgbot(admin, fmt.Sprintf("<b>بررسی پرداخت #%d</b>\nکاربر: %d\nپلن: %s\nمبلغ: %d تومان\nقبل از تأیید، واریز واقعی را بررسی کنید.", order.ID, order.TgID, html.EscapeString(order.PlanName), order.Price), storeReviewKeyboard(order.ID))
	}
	t.SendMsgToTgbot(message.Chat.ID, "رسید ثبت شد؛ نتیجه پس از بررسی مدیر اعلام می‌شود."); return true
}

func (t *Tgbot) storeDecision(admin int64, id int, approve bool) {
	if !checkAdmin(admin) { return }
	s := &service.StoreService{}
	if !approve {
		order, err := s.Reject(id, admin); if err != nil { t.SendMsgToTgbot(admin, html.EscapeString(err.Error())); return }
		t.SendMsgToTgbot(order.TgID, fmt.Sprintf("رسید سفارش #%d تأیید نشد؛ با پشتیبانی تماس بگیرید.", id)); t.SendMsgToTgbot(admin, "سفارش رد شد."); return
	}
	if enabled, err := t.settingService.GetSubEnable(); err != nil || !enabled { t.SendMsgToTgbot(admin, "ابتدا سابسکرایب را فعال کنید."); return }
	order, restart, err := s.Approve(id, admin, &t.inboundService); if restart { t.xrayService.SetToNeedRestart() }
	if err != nil { t.SendMsgToTgbot(admin, "تحویل ناموفق؛ سفارش محفوظ است و می‌توانید دوباره تأیید کنید.\n"+html.EscapeString(err.Error())); return }
	t.SendMsgToTgbot(admin, fmt.Sprintf("سفارش #%d تحویل شد.", id)); t.storeDeliver(order.TgID, order.Email)
}

func (t *Tgbot) storeDeliver(chatID int64, email string) {
	subURL, _, err := t.buildSubscriptionURLs(email)
	if err != nil { t.SendMsgToTgbot(chatID, "لینک اشتراک آماده نیست؛ با پشتیبانی تماس بگیرید."); return }
	t.sendClientSubLinks(chatID, email)
	if png, err := qrcode.Encode(subURL, qrcode.Medium, 320); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second); defer cancel()
		_, _ = bot.SendDocument(ctx, tu.Document(tu.ID(chatID), tu.FileFromBytes(png, "FARSTAR-subscription.png")))
	}
}
