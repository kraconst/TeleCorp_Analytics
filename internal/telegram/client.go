// Package telegram provides the MTProto connection used by the development CLI.
package telegram

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gotd/td/session"
	gotd "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"golang.org/x/net/proxy"
)

type Config struct {
	APIID       int
	APIHash     string
	SessionPath string
	Proxy       string
}

// Client owns one account's connection. Use a separate session path per account.
type Client struct{ client *gotd.Client }

func New(cfg Config) (*Client, error) {
	if cfg.APIID <= 0 || strings.TrimSpace(cfg.APIHash) == "" {
		return nil, fmt.Errorf("TELEGRAM_API_ID must be positive and TELEGRAM_API_HASH must be set")
	}
	if strings.TrimSpace(cfg.SessionPath) == "" {
		return nil, fmt.Errorf("session path must be set")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.SessionPath), 0700); err != nil {
		return nil, fmt.Errorf("create session directory: %w", err)
	}

	opts := gotd.Options{
		SessionStorage: &session.FileStorage{Path: cfg.SessionPath},
		NoUpdates:      true,
	}

	proxyAddr := strings.TrimSpace(cfg.Proxy)
	if proxyAddr != "" {
		dialer, err := createProxyDialer(proxyAddr)
		if err != nil {
			return nil, err
		}
		opts.Resolver = dcs.Plain(dcs.PlainOptions{
			Dial: dialer,
		})
	}

	return &Client{client: gotd.NewClient(cfg.APIID, cfg.APIHash, opts)}, nil
}

func createProxyDialer(proxyAddr string) (dcs.DialFunc, error) {
	if !strings.Contains(proxyAddr, "://") {
		proxyAddr = "http://" + proxyAddr
	}
	proxyURL, err := url.Parse(proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	switch strings.ToLower(proxyURL.Scheme) {
	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("create socks5 dialer: %w", err)
		}
		return func(ctx context.Context, network, addr string) (net.Conn, error) {
			if cd, ok := dialer.(proxy.ContextDialer); ok {
				return cd.DialContext(ctx, network, addr)
			}
			return dialer.Dial(network, addr)
		}, nil

	case "http", "https":
		return func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			conn, err := d.DialContext(ctx, "tcp", proxyURL.Host)
			if err != nil {
				return nil, fmt.Errorf("connect to proxy %s: %w", proxyURL.Host, err)
			}

			req := &http.Request{
				Method: "CONNECT",
				URL:    &url.URL{Opaque: addr},
				Host:   addr,
				Header: make(http.Header),
			}
			if proxyURL.User != nil {
				auth := proxyURL.User.String()
				req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth)))
			}

			if err := req.Write(conn); err != nil {
				conn.Close()
				return nil, fmt.Errorf("send CONNECT to proxy: %w", err)
			}

			resp, err := http.ReadResponse(bufio.NewReader(conn), req)
			if err != nil {
				conn.Close()
				return nil, fmt.Errorf("read CONNECT response: %w", err)
			}
			if resp.StatusCode != http.StatusOK {
				conn.Close()
				return nil, fmt.Errorf("proxy returned status %s", resp.Status)
			}
			return conn, nil
		}, nil

	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q (supported: socks5, http, https)", proxyURL.Scheme)
	}
}

type Profile struct {
	ID       int64
	Name     string
	Username string
}

type Dialog struct {
	ID   int64
	Kind string
	Name string
}

// Inspect restores a session or signs in, then fetches one bounded page of dialogs.
// It closes the connection on return and never logs out the saved session.
func (c *Client) Inspect(ctx context.Context, phone string, authenticator auth.UserAuthenticator, limit int) (Profile, []Dialog, error) {
	var profile Profile
	var dialogs []Dialog
	if limit < 1 || limit > 100 {
		return profile, nil, fmt.Errorf("dialog limit must be between 1 and 100")
	}
	err := c.client.Run(ctx, func(ctx context.Context) error {
		if err := c.client.Auth().IfNecessary(ctx, auth.NewFlow(authenticator, auth.SendCodeOptions{})); err != nil {
			return fmt.Errorf("authorize Telegram account: %w", err)
		}
		self, err := c.client.Self(ctx)
		if err != nil {
			return fmt.Errorf("get own profile: %w", err)
		}
		// Prevent accidentally inspecting another account's saved session.
		if self.Phone != strings.TrimPrefix(phone, "+") {
			return fmt.Errorf("saved session belongs to another phone; use a separate TELEGRAM_SESSION_PATH")
		}
		profile = Profile{ID: self.ID, Name: strings.TrimSpace(self.FirstName + " " + self.LastName), Username: self.Username}
		return c.fetchDialogs(ctx, limit, &dialogs)
	})
	return profile, dialogs, err
}

func is2FARequired(err error) bool {
	if err == nil {
		return false
	}
	return tgerr.Is(err, "SESSION_PASSWORD_NEEDED", "PASSWORD_REQUIRED") ||
		tg.IsPasswordRequired(err) ||
		strings.Contains(err.Error(), "SESSION_PASSWORD_NEEDED")
}

// InspectQR exports a login token (for QR login / tg:// link) and waits for user confirmation on an authorized device.
func (c *Client) InspectQR(ctx context.Context, apiID int, apiHash string, onURL func(string), passwordPrompt func() (string, error), limit int) (Profile, []Dialog, error) {
	var profile Profile
	var dialogs []Dialog
	if limit < 1 || limit > 100 {
		return profile, nil, fmt.Errorf("dialog limit must be between 1 and 100")
	}

	err := c.client.Run(ctx, func(ctx context.Context) error {
		status, err := c.client.Auth().Status(ctx)
		if err != nil || !status.Authorized {
			res, err := c.client.API().AuthExportLoginToken(ctx, &tg.AuthExportLoginTokenRequest{
				APIID:   apiID,
				APIHash: apiHash,
			})
			if err != nil {
				return fmt.Errorf("export login token: %w", err)
			}

			tok, ok := res.(*tg.AuthLoginToken)
			if !ok {
				return fmt.Errorf("unexpected login token response: %T", res)
			}

			link := "tg://login?token=" + base64.RawURLEncoding.EncodeToString(tok.Token)
			if onURL != nil {
				onURL(link)
			}

			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			authorized := false
			for !authorized {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
					res, err := c.client.API().AuthExportLoginToken(ctx, &tg.AuthExportLoginTokenRequest{
						APIID:   apiID,
						APIHash: apiHash,
					})
					if err != nil {
						if is2FARequired(err) {
							if passwordPrompt == nil {
								return fmt.Errorf("2FA password required")
							}
							for {
								pass, err := passwordPrompt()
								if err != nil {
									return fmt.Errorf("read 2FA password: %w", err)
								}
								if _, err := c.client.Auth().Password(ctx, pass); err != nil {
									if errors.Is(err, auth.ErrPasswordInvalid) || strings.Contains(err.Error(), "PASSWORD_HASH_INVALID") {
										fmt.Fprintln(os.Stderr, "Invalid 2FA password, please try again.")
										continue
									}
									return fmt.Errorf("check 2FA password: %w", err)
								}
								authorized = true
								break
							}
							break
						}
						return fmt.Errorf("check login token status: %w", err)
					}

					switch t := res.(type) {
					case *tg.AuthLoginToken:
						newLink := "tg://login?token=" + base64.RawURLEncoding.EncodeToString(t.Token)
						if newLink != link {
							link = newLink
							if onURL != nil {
								onURL(link)
							}
						}
					case *tg.AuthLoginTokenMigrateTo:
						if err := c.client.MigrateTo(ctx, t.DCID); err != nil {
							return fmt.Errorf("migrate to DC %d: %w", t.DCID, err)
						}
						importRes, err := c.client.API().AuthImportLoginToken(ctx, t.Token)
						if err != nil {
							if is2FARequired(err) {
								if passwordPrompt == nil {
									return fmt.Errorf("2FA password required")
								}
								for {
									pass, err := passwordPrompt()
									if err != nil {
										return fmt.Errorf("read 2FA password: %w", err)
									}
									if _, err := c.client.Auth().Password(ctx, pass); err != nil {
										if errors.Is(err, auth.ErrPasswordInvalid) || strings.Contains(err.Error(), "PASSWORD_HASH_INVALID") {
											fmt.Fprintln(os.Stderr, "Invalid 2FA password, please try again.")
											continue
										}
										return fmt.Errorf("check 2FA password: %w", err)
									}
									authorized = true
									break
								}
								break
							}
							return fmt.Errorf("import login token on DC %d: %w", t.DCID, err)
						}
						_ = importRes
						authorized = true
					case *tg.AuthLoginTokenSuccess:
						authorized = true
					}
				}
			}
		}

		self, err := c.client.Self(ctx)
		if err != nil {
			return fmt.Errorf("get own profile: %w", err)
		}
		profile = Profile{ID: self.ID, Name: strings.TrimSpace(self.FirstName + " " + self.LastName), Username: self.Username}
		return c.fetchDialogs(ctx, limit, &dialogs)
	})

	return profile, dialogs, err
}

func (c *Client) fetchDialogs(ctx context.Context, limit int, dialogs *[]Dialog) error {
	result, err := c.client.API().MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{OffsetPeer: &tg.InputPeerEmpty{}, Limit: limit})
	if err != nil {
		return fmt.Errorf("get dialogs: %w", err)
	}
	var items []tg.DialogClass
	var users []tg.UserClass
	var chats []tg.ChatClass
	switch r := result.(type) {
	case *tg.MessagesDialogs:
		items, users, chats = r.Dialogs, r.Users, r.Chats
	case *tg.MessagesDialogsSlice:
		items, users, chats = r.Dialogs, r.Users, r.Chats
	default:
		return fmt.Errorf("unexpected dialogs response: %T", result)
	}
	names := make(map[int64]string)
	for _, u := range users {
		if u, ok := u.(*tg.User); ok {
			names[u.ID] = strings.TrimSpace(u.FirstName + " " + u.LastName)
		}
	}
	chatNames := make(map[int64]string)
	for _, chat := range chats {
		switch chat := chat.(type) {
		case *tg.Chat:
			chatNames[chat.ID] = chat.Title
		case *tg.Channel:
			chatNames[chat.ID] = chat.Title
		}
	}
	for _, item := range items {
		d, ok := item.(*tg.Dialog)
		if !ok {
			continue
		}
		var dialog Dialog
		switch p := d.Peer.(type) {
		case *tg.PeerUser:
			dialog = Dialog{ID: p.UserID, Kind: "user", Name: names[p.UserID]}
		case *tg.PeerChat:
			dialog = Dialog{ID: p.ChatID, Kind: "group", Name: chatNames[p.ChatID]}
		case *tg.PeerChannel:
			dialog = Dialog{ID: p.ChannelID, Kind: "channel", Name: chatNames[p.ChannelID]}
		default:
			continue
		}
		*dialogs = append(*dialogs, dialog)
	}
	return nil
}
