package telegram

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/kraconst/GO_server/internal/config"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

type syncRPCStub struct {
	pins    func(int) (*tg.MessagesPeerDialogs, error)
	dialogs func(*tg.MessagesGetDialogsRequest) (tg.MessagesDialogsClass, error)
	history func(*tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error)
}

func (s syncRPCStub) MessagesGetPinnedDialogs(_ context.Context, folder int) (*tg.MessagesPeerDialogs, error) {
	if s.pins == nil {
		return &tg.MessagesPeerDialogs{}, nil
	}
	return s.pins(folder)
}

func (s syncRPCStub) MessagesGetDialogs(_ context.Context, request *tg.MessagesGetDialogsRequest) (tg.MessagesDialogsClass, error) {
	return s.dialogs(request)
}

func (s syncRPCStub) MessagesGetHistory(_ context.Context, request *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error) {
	return s.history(request)
}

func TestSyncDialogsUsesRawCursorAcrossBothFolders(t *testing.T) {
	user := func(id int64) *tg.User {
		u := &tg.User{ID: id, FirstName: fmt.Sprintf("User %d", id), Username: fmt.Sprintf("user%d", id)}
		u.SetAccessHash(id + 50)
		return u
	}
	peer := func(id int64) *tg.PeerUser { return &tg.PeerUser{UserID: id} }
	dialog := func(id int64, top int) *tg.Dialog {
		return &tg.Dialog{Peer: peer(id), TopMessage: top, UnreadCount: 2}
	}
	bot, deleted, min := user(3), user(5), user(6)
	bot.Bot, deleted.Deleted, min.Min = true, true, true
	channel := &tg.Channel{ID: 44}
	channel.SetAccessHash(120)
	channelPeer := &tg.PeerChannel{ChannelID: 44}
	var pins, calls []int
	reader := &accountReader{limit: 6, selfID: 9, api: syncRPCStub{
		pins: func(folder int) (*tg.MessagesPeerDialogs, error) {
			pins = append(pins, folder)
			id := int64(1 + folder*7)
			return &tg.MessagesPeerDialogs{
				Dialogs: []tg.DialogClass{dialog(id, 2)}, Users: []tg.UserClass{user(id)},
				Messages: []tg.MessageClass{&tg.Message{ID: 2, PeerID: peer(id), Date: 10}},
			}, nil
		},
		dialogs: func(req *tg.MessagesGetDialogsRequest) (tg.MessagesDialogsClass, error) {
			folder, ok := req.GetFolderID()
			if !ok || !req.ExcludePinned || req.Limit != 6 {
				t.Fatalf("expected unpinned dialog request with explicit folder and limit: %#v", req)
			}
			calls = append(calls, folder)
			if req.OffsetID == 0 {
				if _, ok := req.OffsetPeer.(*tg.InputPeerEmpty); !ok || req.OffsetDate != 0 {
					t.Fatalf("first request used a cursor from pinned dialogs: %#v", req)
				}
				if folder == 1 {
					return &tg.MessagesDialogs{Dialogs: []tg.DialogClass{dialog(7, 90), dialog(2, 91)}, Users: []tg.UserClass{user(7), user(2)}}, nil
				}
				return &tg.MessagesDialogsSlice{
					Dialogs: []tg.DialogClass{dialog(2, 72), dialog(3, 71), dialog(5, 70), dialog(6, 69), dialog(9, 67),
						&tg.Dialog{Peer: channelPeer, TopMessage: 66}, &tg.DialogFolder{Peer: peer(8), TopMessage: 2}},
					Users: []tg.UserClass{user(2), bot, deleted, min, user(9)}, Chats: []tg.ChatClass{channel},
					Messages: []tg.MessageClass{&tg.Message{ID: 66, PeerID: peer(3), Date: 910},
						&tg.MessageService{ID: 66, PeerID: channelPeer, Date: 900}},
				}, nil
			}
			if folder != 0 || req.OffsetID != 66 || req.OffsetDate != 900 {
				t.Fatalf("did not use last concrete dialog's top message: %#v", req)
			}
			channel, ok := req.OffsetPeer.(*tg.InputPeerChannel)
			if !ok || channel.ChannelID != 44 || channel.AccessHash != 120 {
				t.Fatalf("wrong channel cursor: %#v", req.OffsetPeer)
			}
			return &tg.MessagesDialogs{Dialogs: []tg.DialogClass{dialog(4, 50)}, Users: []tg.UserClass{user(4)}}, nil
		},
	}}
	got, err := reader.ListDialogs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(got))
	for _, item := range got {
		ids = append(ids, item.TelegramUserID)
		if item.UnreadCount != 2 || item.Name == "" || item.Username == "" {
			t.Fatalf("missing dialog metadata: %#v", item)
		}
	}
	if !slices.Equal(ids, []int64{1, 2, 4, 8, 7}) || !slices.Equal(pins, []int{0, 1}) || !slices.Equal(calls, []int{0, 0, 1}) {
		t.Fatalf("unexpected dialog coverage: IDs %v, pinned folders %v, unpinned requests %v", ids, pins, calls)
	}
	if reader.peers[2].AccessHash != 52 {
		t.Fatal("history peer access hash was not retained in the reader")
	}
}

func TestSyncDialogsRejectsRepeatedCursor(t *testing.T) {
	peer := &tg.PeerChat{ChatID: 7}
	reader := &accountReader{limit: 1, api: syncRPCStub{
		dialogs: func(*tg.MessagesGetDialogsRequest) (tg.MessagesDialogsClass, error) {
			return &tg.MessagesDialogsSlice{Dialogs: []tg.DialogClass{&tg.Dialog{Peer: peer, TopMessage: 8}},
				Messages: []tg.MessageClass{&tg.Message{ID: 8, PeerID: peer, Date: 100}}}, nil
		},
	}}
	if got, err := reader.ListDialogs(t.Context()); err == nil || got != nil || !strings.Contains(err.Error(), "did not advance") {
		t.Fatalf("repeated cursor returned %v, %v", got, err)
	}
}

func TestSyncHistoryKeepsLowerBoundAndMapsMessages(t *testing.T) {
	peer := &tg.InputPeerUser{UserID: 22, AccessHash: 5}
	var offsets []int
	reader := &accountReader{limit: 3, peers: map[int64]*tg.InputPeerUser{22: peer}, api: syncRPCStub{
		history: func(req *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error) {
			if req.Peer != peer || req.MinID != 8 || req.Limit != 3 {
				t.Fatalf("history request lost its bound or peer: %#v", req)
			}
			offsets = append(offsets, req.OffsetID)
			switch req.OffsetID {
			case 0:
				return &tg.MessagesMessagesSlice{Messages: []tg.MessageClass{
					&tg.Message{ID: 14, Date: 104, Out: true, Message: "photo", Media: &tg.MessageMediaPhoto{}},
					&tg.Message{ID: 13, Date: 103, Message: "voice", Media: &tg.MessageMediaDocument{
						Document: &tg.Document{Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeAudio{Voice: true}}}}},
					&tg.MessageEmpty{ID: 12},
				}}, nil
			case 12:
				return &tg.MessagesMessagesSlice{Messages: []tg.MessageClass{
					&tg.Message{ID: 11, Date: 101, Message: "document", Media: &tg.MessageMediaDocument{}},
					&tg.MessageService{ID: 10, Date: 100},
				}}, nil
			case 10:
				return &tg.MessagesMessages{Messages: []tg.MessageClass{
					&tg.Message{ID: 9, Date: 99, Message: "text"}, &tg.Message{ID: 8, Date: 98},
				}}, nil
			default:
				t.Fatalf("unexpected history offset: %d", req.OffsetID)
				return nil, nil
			}
		},
	}}
	dialog := usecase.TelegramDialog{TelegramUserID: 22}
	if _, err := reader.ListHistory(t.Context(), dialog, math.MaxInt32+1); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("unrepresentable Telegram message ID accepted: %v", err)
	}
	got, err := reader.ListHistory(t.Context(), dialog, 8)
	if err != nil || len(got) != 4 || !slices.Equal(offsets, []int{0, 12, 10}) {
		t.Fatalf("history = %v, offsets = %v, error = %v", got, offsets, err)
	}
	if got[0].TelegramMessageID != 9 || got[0].Text != "text" || got[0].MediaType != nil ||
		got[1].TelegramMessageID != 11 || got[1].MediaType == nil || *got[1].MediaType != domain.MediaTypeDocument ||
		got[2].TelegramMessageID != 13 || got[2].MediaType == nil || *got[2].MediaType != domain.MediaTypeVoice ||
		got[3].TelegramMessageID != 14 || got[3].MediaType == nil || *got[3].MediaType != domain.MediaTypePhoto ||
		got[0].Direction != domain.MessageDirectionIn || got[3].Direction != domain.MessageDirectionOut ||
		got[3].SentAt.Location() != time.UTC || got[3].SentAt.Unix() != 104 || got[3].ChatID != 0 {
		t.Fatalf("wrong mapped history: %#v", got)
	}
	if voice := telegramMediaType(&tg.MessageMediaDocument{Voice: true}); voice == nil || *voice != domain.MediaTypeVoice {
		t.Fatal("direct voice flag did not map to voice")
	}
}

func TestSyncHistoryErrorsDoNotExposePartialResults(t *testing.T) {
	readFailure := errors.New("page unavailable")
	reader := &accountReader{limit: 1, peers: map[int64]*tg.InputPeerUser{1: {UserID: 1}}, api: syncRPCStub{
		history: func(req *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error) {
			if req.OffsetID == 0 {
				return &tg.MessagesMessagesSlice{Messages: []tg.MessageClass{&tg.Message{ID: 10, Date: 100}}}, nil
			}
			return nil, readFailure
		},
	}}
	if got, err := reader.ListHistory(t.Context(), usecase.TelegramDialog{TelegramUserID: 1}, 0); got != nil || !errors.Is(err, readFailure) {
		t.Fatalf("failed second page returned %v, %v", got, err)
	}
	if _, err := reader.ListHistory(t.Context(), usecase.TelegramDialog{TelegramUserID: 2}, 0); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("reader queried a peer not obtained through ListDialogs: %v", err)
	}
}

func TestInspectionDialogsKeepsGroupTitleForBothResponseVariants(t *testing.T) {
	dialogs := []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerChat{ChatID: 17}}}
	chats := []tg.ChatClass{&tg.Chat{ID: 17, Title: "group name"}}
	for _, response := range []tg.MessagesDialogsClass{
		&tg.MessagesDialogs{Dialogs: dialogs, Chats: chats},
		&tg.MessagesDialogsSlice{Dialogs: dialogs, Chats: chats},
	} {
		page, err := modifiedDialogs(response)
		if err != nil {
			t.Fatal(err)
		}
		want := []Dialog{{ID: 17, Kind: "group", Name: "group name"}}
		if got := mapInspectionDialogs(page); !slices.Equal(got, want) {
			t.Fatalf("dialog mapping %T: got %v, want %v", response, got, want)
		}
	}
}

func TestTelegramRPCFloodWaitRestartsWithTimeout(t *testing.T) {
	attempts := 0
	got, err := retryTelegramRPC(t.Context(), func(ctx context.Context) (int, error) {
		attempts++
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > syncRPCTimeout {
			t.Fatal("RPC lacks bounds")
		}
		if attempts == 1 {
			return 0, fmt.Errorf("Telegram error: %w", tgerr.New(420, "FLOOD_WAIT_0"))
		}
		return 17, nil
	})
	if err != nil || got != 17 || attempts != 2 {
		t.Fatalf("retry returned %d, %v after %d attempts", got, err, attempts)
	}
}

func TestTelegramRPCFloodWaitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := retryTelegramRPC(ctx, func(context.Context) (int, error) {
			close(started)
			return 0, tgerr.New(420, "FLOOD_WAIT_30")
		})
		finished <- err
	}()
	<-started
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled wait returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("FLOOD_WAIT ignored caller cancellation")
	}

	// HIGH-12: FLOOD_WAIT exceeding maxFloodWait must return immediately without sleeping.
	start := time.Now()
	_, err := retryTelegramRPC(t.Context(), func(context.Context) (int, error) {
		return 0, tgerr.New(420, "FLOOD_WAIT_600")
	})
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("expected immediate error for excessive FLOOD_WAIT, got err=%v in %s", err, time.Since(start))
	}
}

func TestTelegramSessionRejectionStatus(t *testing.T) {
	for methodError, want := range map[string]domain.AccountStatus{
		"SESSION_REVOKED": domain.StatusDisconnected, "SESSION_EXPIRED": domain.StatusDisconnected,
		"AUTH_KEY_UNREGISTERED": domain.StatusDisconnected, "AUTH_KEY_INVALID": domain.StatusDisconnected,
		"AUTH_KEY_DUPLICATED": domain.StatusDisconnected, "USER_DEACTIVATED_BAN": domain.StatusBanned,
		"PHONE_NUMBER_BANNED": domain.StatusBanned, "USER_BANNED_IN_CHANNEL": "", "INTERNAL": "",
	} {
		err := fmt.Errorf("read history: %w", tgerr.New(403, methodError))
		if got := rejectedSessionStatus(err); got != want {
			t.Errorf("%s: got status %q, want %q", methodError, got, want)
		}
	}
	for _, err := range []error{context.DeadlineExceeded, domain.ErrNotFound} {
		if got := rejectedSessionStatus(err); got != "" {
			t.Errorf("%v changed account status to %q", err, got)
		}
	}
	if got := rejectedSessionStatus(errUnreadableAccountSession); got != domain.StatusDisconnected {
		t.Errorf("errUnreadableAccountSession: got status %q, want %q", got, domain.StatusDisconnected)
	}
}

func TestNewSyncClientRequiresCredentialsAndRedactsMalformedProxy(t *testing.T) {
	if _, err := NewSyncClient(config.TelegramConfig{}, &sessionRepositoryStub{}); !errors.Is(err, usecase.ErrTelegramUnavailable) {
		t.Fatalf("empty config returned %v", err)
	}
	const secret = "sample-password"
	config := config.TelegramConfig{
		APIID: 1, APIHash: "test", SessionKey: make([]byte, 32), SyncBatchLimit: 100,
		Proxy: "https://user:" + secret + "@%zz",
	}
	if _, err := NewSyncClient(config, &sessionRepositoryStub{}); !errors.Is(err, domain.ErrValidation) || strings.Contains(err.Error(), secret) {
		t.Fatalf("invalid proxy was accepted or exposed credentials: %v", err)
	}
}

func TestProxyConnectUsesDecodedBasicCredentialsAndHidesResponseText(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response string
		wantErr  string
	}{
		{name: "tunnel", response: "HTTP/1.1 200 Connection Established\r\n\r\nPING"},
		{name: "rejected", response: "HTTP/1.1 407 secret-marker\r\nContent-Length: 0\r\n\r\n", wantErr: "proxy returned HTTP status 407"},
		{name: "malformed", response: "secret-marker\r\n\r\n", wantErr: "read CONNECT response: invalid or incomplete response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			const username, password = "demo@user", "p@ss:with%escapes"
			const destination = "telegram.invalid:443"
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
					done <- err
					return
				}
				req, err := http.ReadRequest(bufio.NewReader(conn))
				if err != nil {
					done <- err
					return
				}
				defer req.Body.Close()
				want := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
				if req.Method != http.MethodConnect || req.Host != destination || req.Header.Get("Proxy-Authorization") != want {
					done <- errors.New("incorrect CONNECT method, target or credentials")
					return
				}
				_, err = io.WriteString(conn, tc.response)
				done <- err
			}()
			address := (&url.URL{Scheme: "http", Host: listener.Addr().String(), User: url.UserPassword(username, password)}).String()
			dial, err := createProxyDialer(address)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			conn, err := dial(ctx, "tcp", destination)
			if conn != nil {
				defer conn.Close()
			}
			select {
			case proxyErr := <-done:
				if proxyErr != nil {
					t.Fatal(proxyErr)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatal("unexpected CONNECT error or unredacted response text")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			var ping [4]byte
			if _, err := io.ReadFull(conn, ping[:]); err != nil || string(ping[:]) != "PING" {
				t.Fatalf("buffered tunnel bytes lost: %v", err)
			}
		})
	}
}

func TestProxyRejectsBadAddressWithoutExposingCredentials(t *testing.T) {
	for _, address := range []string{"http://demo:secret-marker@%zz", "http://demo:secret-marker@", "socks5:///missing-host"} {
		if _, err := createProxyDialer(address); err == nil || strings.Contains(err.Error(), "secret-marker") {
			t.Fatal("bad proxy address accepted or credentials exposed")
		}
	}
}

func TestProxyConnectHonorsCancellationWhileWaiting(t *testing.T) {
	requested := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requested)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	defer close(release)
	dial, err := createProxyDialer(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		conn, err := dial(ctx, "tcp", "telegram.invalid:443")
		if conn != nil {
			_ = conn.Close()
		}
		result <- err
	}()
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("proxy never received CONNECT request")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("CONNECT cancellation returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled CONNECT handshake did not stop")
	}
}

func TestAccountReaderListHistoryStopsAtInitialDaysCutoff(t *testing.T) {
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	recentDate := int(now.AddDate(0, 0, -5).Unix())
	oldDate := int(now.AddDate(0, 0, -45).Unix())
	calls := 0
	rpc := syncRPCStub{
		history: func(req *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error) {
			calls++
			if calls > 1 {
				t.Fatal("pagination should stop once cutoff date is reached")
			}
			return &tg.MessagesMessagesSlice{
				Count: 2,
				Messages: []tg.MessageClass{
					&tg.Message{ID: 10, Date: recentDate, Message: "recent"},
					&tg.Message{ID: 5, Date: oldDate, Message: "too old"},
				},
			}, nil
		},
	}
	reader := &accountReader{
		api:         rpc,
		limit:       2,
		initialDays: 30,
		now:         func() time.Time { return now },
		peers:       map[int64]*tg.InputPeerUser{42: {UserID: 42, AccessHash: 1}},
	}
	msgs, err := reader.ListHistory(t.Context(), usecase.TelegramDialog{TelegramUserID: 42}, 0)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if calls != 1 || len(msgs) != 1 || msgs[0].TelegramMessageID != 10 {
		t.Fatalf("expected 1 recent message in 1 call, got calls=%d msgs=%+v", calls, msgs)
	}
}

func TestAccountReaderListHistorySkipsOldDialogByTopDate(t *testing.T) {
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	oldDate := now.AddDate(0, 0, -60).Unix()
	rpc := syncRPCStub{
		history: func(*tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error) {
			t.Fatal("MessagesGetHistory must not be called for dialog whose top message is older than cutoff")
			return nil, nil
		},
	}
	reader := &accountReader{
		api:         rpc,
		limit:       50,
		initialDays: 30,
		now:         func() time.Time { return now },
		peers:       map[int64]*tg.InputPeerUser{42: {UserID: 42, AccessHash: 1}},
		topDates:    map[int64]int64{42: oldDate},
	}
	msgs, err := reader.ListHistory(t.Context(), usecase.TelegramDialog{TelegramUserID: 42}, 0)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected empty slice, got %+v", msgs)
	}
}
