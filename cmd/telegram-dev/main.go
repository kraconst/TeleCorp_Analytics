package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/skip2/go-qrcode"
	"golang.org/x/term"

	"github.com/kraconst/GO_server/internal/telegram"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Telegram:", err)
		os.Exit(1)
	}
}

func run() error {
	loadDotEnv()

	limit := flag.Int("limit", 20, "maximum number of dialogs (1–100)")
	noProxy := flag.Bool("no-proxy", false, "disable proxy and connect directly (e.g. through system VPN)")
	qrMode := flag.Bool("qr", false, "log in via QR code / tg:// link without waiting for SMS or in-app code")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *limit < 1 || *limit > 100 {
		return errors.New("-limit must be between 1 and 100")
	}
	id, err := strconv.Atoi(os.Getenv("TELEGRAM_API_ID"))
	if err != nil || id <= 0 {
		return errors.New("set TELEGRAM_API_ID to a positive integer in .env or environment")
	}

	useProxyEnv := strings.ToLower(strings.TrimSpace(os.Getenv("USE_PROXY")))
	proxyDisabled := *noProxy || useProxyEnv == "false" || useProxyEnv == "0" || strings.ToLower(strings.TrimSpace(os.Getenv("TELEGRAM_NO_PROXY"))) == "true"

	proxyAddr := ""
	if !proxyDisabled {
		proxyAddr = strings.TrimSpace(os.Getenv("TELEGRAM_PROXY"))
		if proxyAddr == "" {
			proxyAddr = strings.TrimSpace(os.Getenv("ALL_PROXY"))
		}
		if proxyAddr == "" {
			proxyAddr = strings.TrimSpace(os.Getenv("all_proxy"))
		}
	}

	if proxyAddr != "" {
		fmt.Printf("Using proxy: %s\n", proxyAddr)
	} else {
		fmt.Println("Proxy disabled: connecting directly (via system network / VPN)...")
	}

	if *qrMode {
		path := os.Getenv("TELEGRAM_SESSION_PATH")
		if path == "" {
			path = "./var/telegram/account-a.session"
		}
		client, err := telegram.New(telegram.Config{
			APIID:       id,
			APIHash:     os.Getenv("TELEGRAM_API_HASH"),
			SessionPath: path,
			Proxy:       proxyAddr,
		})
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		fmt.Println("Connecting to Telegram for QR login...")
		onURL := func(link string) {
			fmt.Printf("\n============================================================\n")
			fmt.Printf("SCAN QR CODE WITH TELEGRAM ON YOUR PHONE:\n")
			fmt.Printf("1. Open Telegram on your PHONE\n")
			fmt.Printf("2. Go to: Settings -> Devices -> Link Desktop Device\n")
			fmt.Printf("   (Настройки -> Устройства -> Подключить устройство)\n")
			fmt.Printf("3. Point your camera at the QR code below\n")
			fmt.Printf("============================================================\n\n")

			qrObj, err := qrcode.New(link, qrcode.Medium)
			if err == nil {
				// Print terminal QR code using half-block Unicode characters
				fmt.Print(qrObj.ToSmallString(false))

				// Generate an HTML page with the QR code and open it in the default browser
				pngBytes, err := qrObj.PNG(512)
				if err == nil {
					b64PNG := base64.StdEncoding.EncodeToString(pngBytes)
					htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Telegram Device Login</title>
<style>
  * { box-sizing: border-box; }
  body {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    margin: 0;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    background: #0e1621;
    color: #ffffff;
  }
  .card {
    background: #17212b;
    border: 1px solid #242f3d;
    padding: 36px 32px;
    border-radius: 16px;
    box-shadow: 0 16px 40px rgba(0,0,0,0.5);
    text-align: center;
    max-width: 420px;
    width: 90%%;
  }
  h2 { margin: 0 0 16px 0; font-size: 22px; color: #ffffff; }
  .qr-box {
    background: #ffffff;
    padding: 16px;
    border-radius: 12px;
    display: inline-block;
    margin-bottom: 24px;
  }
  img { width: 280px; height: 280px; display: block; }
  ol {
    text-align: left;
    margin: 0;
    padding-left: 20px;
    font-size: 15px;
    color: #798b9b;
    line-height: 1.6;
  }
  li { margin-bottom: 10px; }
  b { color: #5288c1; }
  .hint {
    margin-top: 18px;
    font-size: 13px;
    color: #6c7883;
  }
</style>
</head>
<body>
<div class="card">
  <h2>Telegram Authorization</h2>
  <div class="qr-box">
    <img src="data:image/png;base64,%s" alt="Telegram QR Code" />
  </div>
  <ol>
    <li>Open Telegram on your <b>phone</b></li>
    <li>Go to <b>Settings &rarr; Devices &rarr; Link Desktop Device</b><br><span style="color:#6c7883; font-size:13px;">(Настройки &rarr; Устройства &rarr; Подключить устройство)</span></li>
    <li>Scan this QR code with your phone camera</li>
  </ol>
  <div class="hint">Waiting for confirmation...</div>
</div>
</body>
</html>`, b64PNG)
					dir := filepath.Dir(path)
					_ = os.MkdirAll(dir, 0700)
					htmlPath := filepath.Join(dir, "qr_login.html")
					if err := os.WriteFile(htmlPath, []byte(htmlContent), 0600); err == nil {
						absHTML, _ := filepath.Abs(htmlPath)
						fmt.Printf("Opened QR login page: %s\n", absHTML)
						_ = exec.Command("cmd", "/c", "start", "", absHTML).Start()
					}
				}
			} else {
				fmt.Printf("Login URL: %s\n", link)
			}
			fmt.Printf("\nWaiting for confirmation from your phone...\n")
		}

		passwordPrompt := func() (string, error) {
			return secret("2FA password: ")
		}

		profile, dialogs, err := client.InspectQR(ctx, id, os.Getenv("TELEGRAM_API_HASH"), onURL, passwordPrompt, *limit)
		if err != nil {
			return err
		}
		fmt.Printf("Connected: %q (@%s), ID %d\nSession: %s\n", profile.Name, profile.Username, profile.ID, path)
		fmt.Printf("Dialogs (up to %d; first page):\n", *limit)
		for _, d := range dialogs {
			fmt.Printf("%s\t%d\t%q\n", d.Kind, d.ID, d.Name)
		}
		return nil
	}

	phone := strings.TrimSpace(os.Getenv("TELEGRAM_PHONE"))
	if phone == "" && term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Enter phone number (+79991234567): ")
		var input string
		fmt.Scanln(&input)
		phone = strings.TrimSpace(input)
	}
	if !validPhone(phone) {
		return errors.New("set TELEGRAM_PHONE in international format, e.g. +79991234567 in .env or environment")
	}
	path := os.Getenv("TELEGRAM_SESSION_PATH")
	if path == "" {
		path = "./var/telegram/" + strings.TrimPrefix(phone, "+") + ".session"
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("run this command in an interactive terminal")
	}

	client, err := telegram.New(telegram.Config{
		APIID:       id,
		APIHash:     os.Getenv("TELEGRAM_API_HASH"),
		SessionPath: path,
		Proxy:       proxyAddr,
	})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Printf("Connecting to Telegram (phone: %s)...\n", phone)
	profile, dialogs, err := client.Inspect(ctx, phone, &prompts{phone: phone}, *limit)
	if err != nil {
		return err
	}
	fmt.Printf("Connected: %q (@%s), ID %d\nSession: %s\n", profile.Name, profile.Username, profile.ID, path)
	fmt.Printf("Dialogs (up to %d; first page):\n", *limit)
	for _, d := range dialogs {
		fmt.Printf("%s\t%d\t%q\n", d.Kind, d.ID, d.Name)
	}
	return nil
}

func loadDotEnv() {
	data, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			if os.Getenv(k) == "" && v != "" {
				os.Setenv(k, v)
			}
		}
	}
}

func validPhone(phone string) bool {
	if len(phone) < 8 || len(phone) > 16 || phone[0] != '+' {
		return false
	}
	for _, r := range phone[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type prompts struct{ phone string }

func (p *prompts) Phone(context.Context) (string, error) { return p.phone, nil }
func (p *prompts) Code(_ context.Context, code *tg.AuthSentCode) (string, error) {
	fmt.Fprintf(os.Stderr, "Telegram selected code delivery: %T\n", code.Type)
	return secret("Login code: ")
}
func (p *prompts) Password(context.Context) (string, error) { return secret("2FA password: ") }
func (p *prompts) SignUp(context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("create the Telegram account in an official app first")
}
func (p *prompts) AcceptTermsOfService(context.Context, tg.HelpTermsOfService) error {
	return errors.New("account registration is not supported; use an existing Telegram account")
}

func secret(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return "", errors.New("input is empty")
		}
		return line, nil
	}
	value, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	defer func() {
		for i := range value {
			value[i] = 0
		}
	}()
	if len(value) == 0 || strings.IndexFunc(string(value), unicode.IsControl) >= 0 {
		return "", errors.New("input is empty or contains control characters")
	}
	return string(value), nil
}
