// Package telegram provides the MTProto connection used by the development CLI.
package telegram

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
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
	"github.com/gotd/td/telegram/auth/qrlogin"
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
type Client struct {
	client     *gotd.Client
	dispatcher tg.UpdateDispatcher
}

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

	dispatcher := tg.NewUpdateDispatcher()
	opts := gotd.Options{
		SessionStorage: &session.FileStorage{Path: cfg.SessionPath},
		UpdateHandler:  dispatcher,
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

	return &Client{
		client:     gotd.NewClient(cfg.APIID, cfg.APIHash, opts),
		dispatcher: dispatcher,
	}, nil
}

type bufferedConn struct {
	net.Conn
	r io.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) {
	return b.r.Read(p)
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
		isTLS := strings.ToLower(proxyURL.Scheme) == "https"
		return func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			conn, err := d.DialContext(ctx, "tcp", proxyURL.Host)
			if err != nil {
				return nil, fmt.Errorf("connect to proxy %s: %w", proxyURL.Host, err)
			}

			if isTLS {
				tlsConn := tls.Client(conn, &tls.Config{
					ServerName: proxyURL.Hostname(),
				})
				if err := tlsConn.HandshakeContext(ctx); err != nil {
					conn.Close()
					return nil, fmt.Errorf("tls handshake to proxy %s: %w", proxyURL.Host, err)
				}
				conn = tlsConn
			}

			// Handshake deadline to prevent hanging connections
			deadline := time.Now().Add(15 * time.Second)
			if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
				deadline = d
			}
			_ = conn.SetDeadline(deadline)

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

			bufReader := bufio.NewReader(conn)
			resp, err := http.ReadResponse(bufReader, req)
			if err != nil {
				conn.Close()
				return nil, fmt.Errorf("read CONNECT response: %w", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				conn.Close()
				return nil, fmt.Errorf("proxy returned status %s", resp.Status)
			}

			// Clear deadline after handshake
			_ = conn.SetDeadline(time.Time{})

			return &bufferedConn{
				Conn: conn,
				r:    bufReader,
			}, nil
		}, nil

	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q (supported: socks5, socks5h, http, https)", proxyURL.Scheme)
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

// InspectQR exports a login token (for QR login) and waits for user confirmation on an authorized device.
func (c *Client) InspectQR(ctx context.Context, onURL func(string), passwordPrompt func() (string, error), limit int) (Profile, []Dialog, error) {
	var profile Profile
	var dialogs []Dialog
	if limit < 1 || limit > 100 {
		return profile, nil, fmt.Errorf("dialog limit must be between 1 and 100")
	}

	err := c.client.Run(ctx, func(ctx context.Context) error {
		status, err := c.client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("check auth status: %w", err)
		}

		if !status.Authorized {
			loggedIn := qrlogin.OnLoginToken(c.dispatcher)
			show := func(ctx context.Context, token qrlogin.Token) error {
				if onURL != nil {
					onURL(token.URL())
				}
				return nil
			}

			_, err := c.client.QR().Auth(ctx, loggedIn, show)
			if err != nil {
				if is2FARequired(err) {
					if passwordPrompt == nil {
						return fmt.Errorf("2FA password required")
					}
					pass, err := passwordPrompt()
					if err != nil {
						return fmt.Errorf("read 2FA password: %w", err)
					}
					if _, err := c.client.Auth().Password(ctx, pass); err != nil {
						return fmt.Errorf("check 2FA password: %w", err)
					}
				} else {
					return fmt.Errorf("QR login: %w", err)
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

	titleByPeer := make(map[string]string)
	switch r := result.(type) {
	case *tg.MessagesDialogs:
		for _, u := range r.Users {
			if user, ok := u.(*tg.User); ok {
				titleByPeer[fmt.Sprintf("user_%d", user.ID)] = strings.TrimSpace(user.FirstName + " " + user.LastName)
			}
		}
		for _, ch := range r.Chats {
			switch c := ch.(type) {
			case *tg.Chat:
				titleByPeer[fmt.Sprintf("chat_%d", c.ID)] = c.Title
			case *tg.Channel:
				titleByPeer[fmt.Sprintf("channel_%d", c.ID)] = c.Title
			}
		}
		for _, d := range r.Dialogs {
			if diag, ok := d.(*tg.Dialog); ok {
				var id int64
				var kind string
				switch p := diag.Peer.(type) {
				case *tg.PeerUser:
					id = p.UserID
					kind = "user"
				case *tg.PeerChat:
					id = p.ChatID
					kind = "group"
				case *tg.PeerChannel:
					id = p.ChannelID
					kind = "channel"
				}
				name := titleByPeer[fmt.Sprintf("%s_%d", kind, id)]
				if name == "" {
					name = fmt.Sprintf("ID %d", id)
				}
				*dialogs = append(*dialogs, Dialog{ID: id, Kind: kind, Name: name})
			}
		}
	case *tg.MessagesDialogsSlice:
		for _, u := range r.Users {
			if user, ok := u.(*tg.User); ok {
				titleByPeer[fmt.Sprintf("user_%d", user.ID)] = strings.TrimSpace(user.FirstName + " " + user.LastName)
			}
		}
		for _, ch := range r.Chats {
			switch c := ch.(type) {
			case *tg.Chat:
				titleByPeer[fmt.Sprintf("chat_%d", c.ID)] = c.Title
			case *tg.Channel:
				titleByPeer[fmt.Sprintf("channel_%d", c.ID)] = c.Title
			}
		}
		for _, d := range r.Dialogs {
			if diag, ok := d.(*tg.Dialog); ok {
				var id int64
				var kind string
				switch p := diag.Peer.(type) {
				case *tg.PeerUser:
					id = p.UserID
					kind = "user"
				case *tg.PeerChat:
					id = p.ChatID
					kind = "group"
				case *tg.PeerChannel:
					id = p.ChannelID
					kind = "channel"
				}
				name := titleByPeer[fmt.Sprintf("%s_%d", kind, id)]
				if name == "" {
					name = fmt.Sprintf("ID %d", id)
				}
				*dialogs = append(*dialogs, Dialog{ID: id, Kind: kind, Name: name})
			}
		}
	}
	return nil
}
