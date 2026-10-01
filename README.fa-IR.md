# FARSTAR PANEL

پنل گرافیکی فارسی و راست‌چین برای مدیریت زیرساخت‌های Xray/3x-ui-compatible. این نسخه یک **نسخه نمایشی قابل اجرا** است که معماری رابط، مدل داده و نقاط ورود عملیات اصلی را آماده می‌کند و در حالت `demo` بدون وابستگی خارجی اجرا می‌شود.

## امکانات فعلی

- داشبورد گرافیکی با آمار آنلاین، مصرف ترافیک، وضعیت نودها و توزیع پروتکل‌ها
- مدیریت کاربران با سهمیه، تاریخ انقضا، Limit IP، پروتکل و نود
- ضریب مصرف برای هر کاربر و ضریب پیش‌فرض سیستم؛ مثال `×1.5` یعنی هر ۱ گیگابایت خام، ۱.۵ گیگابایت از سهمیه کم می‌کند
- صفحات مدیریت اینباند، سابسکریپشن، نودها، روتینگ و خروجی، قالب‌ها، اعلان‌ها، بکاپ و تنظیمات
- استودیو لینک سابسکریپشن با برندینگ، QR، خروجی Auto / Raw / Clash / JSON و پیش‌نمایش
- تم تیره، RTL، ریسپانسیو موبایل، جست‌وجوی بالای صفحه و toastهای عملیاتی
- سرور Node.js کوچک با endpoint سلامت `/api/health`

## تطبیق قابلیت‌ها با 3X-UI Sanaei

فهرست مرجع این پروژه بر اساس قابلیت‌های اعلام‌شده در مستندات رسمی 3X-UI تنظیم شده است: پشتیبانی از VLESS، VMess، Trojan، Shadowsocks، WireGuard، AmneziaWG، TUIC، Hysteria2، MTProto، HTTP، SOCKS و Dokodemo-door/TUN؛ ترنسپورت‌های TCP، mKCP، WebSocket، gRPC، HTTPUpgrade و XHTTP؛ امنیت TLS، XTLS و REALITY؛ مدیریت سهمیه/انقضا/IP؛ لینک و QR؛ آمار ترافیک؛ multi-node؛ routing/outbound؛ subscription؛ ربات‌ها؛ REST API؛ SQLite/PostgreSQL؛ تم روشن/تیره؛ PWA و fail2ban.

در UI این نسخه، بخش‌های اصلی این قابلیت‌ها با منوی مجزا و مدل داده آماده شده‌اند. اتصال عملیاتی به Xray و API پنل اصلی باید در لایه adapter سمت سرور اضافه شود و عمداً از داخل مرورگر انجام نمی‌شود تا کلید API لو نرود.

## اجرا

نیازمندی: Node.js نسخه ۲۰ یا بالاتر.

```bash
npm start
```

سپس مرورگر را روی [http://localhost:4173](http://localhost:4173) باز کن.

برای تغییر پورت:

```powershell
$env:PORT=8080
npm start
```

## اتصال به محیط واقعی

1. یک adapter سمت سرور برای API رسمی 3x-ui بنویس.
2. توکن API را فقط در متغیر محیطی/secret manager نگه دار؛ آن را در `app.js` یا مرورگر قرار نده.
3. عملیات‌های `list inbounds`، `list clients`، `create/update client`، reset traffic، subscription و node health را به endpointهای server-side وصل کن.
4. برای production، HTTPS، احراز هویت، CSRF protection، rate limit، audit log و backup رمزگذاری‌شده را فعال کن.

## انتشار روی GitHub

این پوشه در ابتدا ریپوی Git نبود. بعد از ساخت ریپوی خالی در GitHub، این دستورات را اجرا کن:

```bash
git init
git add .
git commit -m "feat: create FARSTAR PANEL RTL dashboard"
git branch -M main
git remote add origin https://github.com/YOUR_USERNAME/farstar-panel.git
git push -u origin main
```

## توجه امنیتی

این پروژه برای مدیریت سرویس‌های شخصی و مجاز طراحی شده است. کلیدها، اطلاعات کاربران و توکن API را commit نکن و پیش از استفاده عمومی، احراز هویت و کنترل دسترسی سمت سرور را کامل کن.

## منابع مرجع

- [مستندات رسمی 3X-UI](https://docs.sanaei.dev/)
- [مخزن رسمی MHSanaei/3x-ui](https://github.com/MHSanaei/3x-ui)
