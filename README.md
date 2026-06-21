<div align="center">

# فارستار تانل پنل

پنل فارسی، متن‌باز و Self-Hosted برای ساخت و مدیریت تانل‌های Reverse TCP و WSS

[![CI](https://github.com/farstar-team/panel/actions/workflows/ci.yml/badge.svg)](https://github.com/farstar-team/panel/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/farstar-team/panel)](https://github.com/farstar-team/panel/releases)
[![License](https://img.shields.io/github/license/farstar-team/panel)](LICENSE)

</div>

## معرفی

فارستار یک پنل مستقل برای اتصال امن و پایدار دو یا چند سرور است. پنل، موتور تانل، پایگاه داده، رابط وب و مدیریت سرویس را در یک باینری ارائه می‌کند و برای اجرا به Docker، Node.js یا دیتابیس خارجی نیاز ندارد.

کاربرد متداول:

- سرور خارج با نقش `Server`
- سرور ایران با نقش `Client`
- اتصال پورت‌های عمومی سرور خارج به سرویس‌های محلی سرور ایران

## قابلیت‌ها

- رابط فارسی RTL و واکنش‌گرا
- پوسته تاریک و روشن
- TCP Mux با سربار کم
- WSS Mux روی TLS
- چند پورت و چند سرویس در هر تانل
- شروع، توقف و راه‌اندازی مجدد از پنل
- Autostart پس از reboot
- آمار زنده ترافیک و اتصال‌ها
- نمایش لاگ هر تانل
- بکاپ JSON تنظیمات
- ذخیره رازهای تانل با AES-256-GCM
- هش رمز مدیر با bcrypt
- نشست HttpOnly و SameSite
- محافظت CSRF
- محدودیت تلاش ورود
- احراز هویت Challenge/HMAC در TCP
- اعتبارسنجی TLS به‌صورت پیش‌فرض
- پشتیبانی از CA اختصاصی
- اجرای پنل با کاربر محدود systemd
- پشتیبانی از Linux AMD64 و ARM64

## سیستم‌عامل‌های پیشنهادی

- Ubuntu 22.04 و 24.04
- Debian 12
- AlmaLinux 9
- Rocky Linux 9

سرور باید systemd و دسترسی root داشته باشد.

## نصب سریع

روی هر دو سرور دستور زیر را اجرا کنید:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/farstar-team/panel/main/install.sh)
```

نصب‌کننده:

1. معماری CPU را تشخیص می‌دهد.
2. آخرین Release را دانلود می‌کند.
3. checksum باینری را بررسی می‌کند.
4. کاربر محدود `farstar` می‌سازد.
5. دیتابیس و master key را در `/etc/farstar` ایجاد می‌کند.
6. سرویس systemd را فعال می‌کند.

نصب به یک GitHub Release معتبر نیاز دارد و برای پایداری، روی سرور مقصد
سورس را build یا ابزار Go را دانلود نمی‌کند.

در نصب اولیه پورت پنل، آدرس bind، نام کاربری و رمز مدیر پرسیده می‌شود.

### دسترسی امن پیش‌فرض

آدرس پیش‌فرض پنل `127.0.0.1:8088` است. برای دسترسی از سیستم شخصی:

```bash
ssh -L 8088:127.0.0.1:8088 root@SERVER_IP
```

سپس باز کنید:

```text
http://127.0.0.1:8088
```

برای دسترسی عمومی، استفاده از HTTPS با Caddy یا Nginx توصیه می‌شود.

## نصب غیرتعاملی

برای automation می‌توان تنظیمات را با متغیر محیطی ارسال کرد:

```bash
export FARSTAR_PANEL_PORT=8088
export FARSTAR_PANEL_BIND=127.0.0.1
export FARSTAR_ADMIN_USER=admin
export FARSTAR_ADMIN_PASSWORD='A-Long-Random-Password'

bash <(curl -fsSL https://raw.githubusercontent.com/farstar-team/panel/main/install.sh)
```

مقادیر مجاز `FARSTAR_PANEL_BIND`:

- `127.0.0.1` دسترسی محلی و امن‌تر
- `0.0.0.0` دسترسی روی تمام IPv4ها

## ساخت اولین تانل TCP

### ۱. سرور خارج

در پنل سرور خارج:

- نقش: `Server`
- پروتکل: `TCP Mux`
- آدرس شنود: `0.0.0.0:443`
- پورت عمومی: برای مثال `0.0.0.0:8443`
- راز مشترک: یک مقدار تصادفی حداقل ۱۶ کاراکتری

تانل را ذخیره و Start کنید.

### ۲. سرور ایران

در پنل سرور ایران:

- نقش: `Client`
- پروتکل: `TCP Mux`
- آدرس سرور: `IP_SERVER:443`
- سرویس محلی: برای مثال `127.0.0.1:8080`
- راز مشترک: دقیقاً همان راز سرور

تانل را ذخیره و Start کنید. اکنون اتصال به پورت `8443` سرور خارج به سرویس `127.0.0.1:8080` سرور ایران منتقل می‌شود.

## چند پورت

در سرور، هر listener را در یک خط وارد کنید:

```text
0.0.0.0:8443
0.0.0.0:2053
0.0.0.0:2083
```

در کلاینت، سرویس‌های متناظر را با همان ترتیب وارد کنید:

```text
127.0.0.1:8080
127.0.0.1:8081
127.0.0.1:8082
```

ترتیب دو فهرست اهمیت دارد.

## تانل WSS

برای سرور WSS مسیر گواهی و کلید TLS لازم است:

```text
/etc/letsencrypt/live/tunnel.example.com/fullchain.pem
/etc/letsencrypt/live/tunnel.example.com/privkey.pem
```

آدرس کلاینت:

```text
wss://tunnel.example.com/tunnel
```

اگر از گواهی خصوصی استفاده می‌کنید، مسیر CA را در کلاینت وارد کنید. گزینه `Skip TLS Verify` فقط برای عیب‌یابی کوتاه‌مدت است.

کاربر `farstar` باید اجازه خواندن فایل‌های certificate و private key را داشته باشد.

## HTTPS برای پنل با Caddy

نمونه Caddyfile:

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8088
}
```

پس از فعال شدن HTTPS، فایل سرویس را ویرایش کنید:

```bash
sudo systemctl edit farstar
```

و اضافه کنید:

```ini
[Service]
Environment=FARSTAR_COOKIE_SECURE=true
```

سپس:

```bash
sudo systemctl daemon-reload
sudo systemctl restart farstar
```

## بروزرسانی

همان دستور نصب را دوباره اجرا کنید:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/farstar-team/panel/main/install.sh)
```

دیتابیس، master key، تانل‌ها و لاگ‌ها حفظ می‌شوند.

برای نصب یک Release مشخص:

```bash
export FARSTAR_VERSION=v0.1.0
bash <(curl -fsSL https://raw.githubusercontent.com/farstar-team/panel/main/install.sh)
```

## مدیریت سرویس

```bash
systemctl status farstar
systemctl restart farstar
systemctl stop farstar
journalctl -u farstar -f
```

مسیرهای مهم:

```text
/usr/local/bin/farstar       باینری
/etc/farstar/farstar.db      دیتابیس
/etc/farstar/master.key      کلید رمزگذاری
/etc/farstar/logs/           لاگ تانل‌ها
/etc/systemd/system/farstar.service
```

از `master.key` و دیتابیس با هم نسخه پشتیبان نگه دارید. بدون master key امکان رمزگشایی رازهای ذخیره‌شده وجود ندارد.

## حذف

حذف برنامه با حفظ داده‌ها:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/farstar-team/panel/main/uninstall.sh)
```

حذف کامل و غیرقابل‌بازگشت:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/farstar-team/panel/main/uninstall.sh) --purge
```

## Build از سورس

Go 1.24 یا جدیدتر:

```bash
git clone https://github.com/farstar-team/panel.git
cd panel
go test ./...
CGO_ENABLED=0 go build -trimpath -o farstar ./cmd/farstar
```

اجرای توسعه:

```bash
mkdir -p ./data
printf 'A-Long-Random-Password' |
  FARSTAR_DATA_DIR=./data ./farstar setup --username admin --password-stdin

FARSTAR_DATA_DIR=./data \
FARSTAR_LISTEN=127.0.0.1:8088 \
./farstar serve
```

## تنظیمات محیطی

| متغیر | پیش‌فرض | توضیح |
|---|---|---|
| `FARSTAR_DATA_DIR` | `/etc/farstar` | مسیر دیتابیس، کلید و لاگ |
| `FARSTAR_LISTEN` | `127.0.0.1:8080` | آدرس پنل |
| `FARSTAR_TLS_CERT` | خالی | گواهی TLS داخلی پنل |
| `FARSTAR_TLS_KEY` | خالی | کلید TLS داخلی پنل |
| `FARSTAR_COOKIE_SECURE` | `false` | الزام ارسال cookie روی HTTPS |

## امنیت

مشکلات امنیتی را عمومی گزارش نکنید. روش گزارش خصوصی در [SECURITY.md](SECURITY.md) توضیح داده شده است.

## مستندات توسعه و عیب‌یابی

- [معماری پروژه](docs/ARCHITECTURE.md)
- [راهنمای عیب‌یابی](docs/TROUBLESHOOTING.md)
- [فرآیند انتشار نسخه](docs/RELEASE.md)
- [راهنمای مشارکت](CONTRIBUTING.md)

## مجوز

این پروژه تحت مجوز MIT منتشر شده است.
