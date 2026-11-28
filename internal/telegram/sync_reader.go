package telegram

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

type accountReader struct {
	api         syncRPC
	limit       int
	initialDays int
	now         func() time.Time
	selfID      int64
	peers       map[int64]*tg.InputPeerUser
	topDates    map[int64]int64
}

func (r *accountReader) ListDialogs(ctx context.Context) ([]usecase.TelegramDialog, error) {
	result := make([]usecase.TelegramDialog, 0)
	peers := make(map[int64]*tg.InputPeerUser)
	topDates := make(map[int64]int64)
	collect := func(dialogs []tg.DialogClass, users map[int64]*tg.User, messages []tg.MessageClass) {
		msgDates := make(map[dialogTopKey]int64, len(messages))
		for _, entry := range messages {
			msg, ok := entry.AsNotEmpty()
			if !ok || msg.GetDate() <= 0 {
				continue
			}
			msgDates[dialogTopKey{peer: dialogPeerKey(msg.GetPeerID()), id: msg.GetID()}] = int64(msg.GetDate())
		}
		for _, item := range dialogs {
			dialog, ok := item.(*tg.Dialog)
			if !ok {
				continue
			}
			peer, ok := dialog.Peer.(*tg.PeerUser)
			if !ok || peer.UserID <= 0 || peer.UserID == r.selfID {
				continue
			}
			user := users[peer.UserID]
			if user == nil || user.Bot || user.Deleted || user.Self || user.Min || peers[peer.UserID] != nil {
				continue
			}
			hash, ok := user.GetAccessHash()
			if !ok {
				// Without a full hash Telegram cannot read this peer's history.
				continue
			}
			peers[peer.UserID] = &tg.InputPeerUser{UserID: peer.UserID, AccessHash: hash}
			var lastMsgAt time.Time
			if topDate := msgDates[dialogTopKey{peer: telegramPeerKey{kind: "user", id: peer.UserID}, id: dialog.GetTopMessage()}]; topDate > 0 {
				topDates[peer.UserID] = topDate
				lastMsgAt = time.Unix(topDate, 0).UTC()
			}
			result = append(result, usecase.TelegramDialog{
				TelegramUserID: peer.UserID, Name: telegramUserName(user), Username: user.Username, UnreadCount: dialog.UnreadCount, LastMessageAt: lastMsgAt,
			})
		}
	}
	// Telegram represents the main list and the archive as peer folders 0 and 1.
	// Pins are ordered independently of date, so they cannot seed the page cursor.
	for _, folder := range []int{0, 1} {
		pinned, err := retryTelegramRPC(ctx, func(ctx context.Context) (*tg.MessagesPeerDialogs, error) {
			return r.api.MessagesGetPinnedDialogs(ctx, folder)
		})
		if err != nil {
			return nil, fmt.Errorf("read pinned Telegram dialogs: %w", err)
		}
		if pinned == nil {
			return nil, errors.New("Telegram returned no pinned dialogs response")
		}
		collect(pinned.Dialogs, pinned.MapUsers().UserToMap(), pinned.Messages)
		if err := r.readUnpinnedDialogs(ctx, folder, collect); err != nil {
			return nil, err
		}
	}
	r.peers = peers
	r.topDates = topDates
	return result, nil
}

type dialogTopKey struct {
	peer telegramPeerKey
	id   int
}

func (r *accountReader) readUnpinnedDialogs(ctx context.Context, folder int, collect func([]tg.DialogClass, map[int64]*tg.User, []tg.MessageClass)) error {
	seen := make(map[dialogCursor]struct{})
	request := tg.MessagesGetDialogsRequest{OffsetPeer: &tg.InputPeerEmpty{}, Limit: r.limit, ExcludePinned: true}
	request.SetFolderID(folder)
	for {
		response, err := retryTelegramRPC(ctx, func(ctx context.Context) (tg.MessagesDialogsClass, error) {
			return r.api.MessagesGetDialogs(ctx, &request)
		})
		if err != nil {
			return fmt.Errorf("read Telegram dialogs: %w", err)
		}
		page, err := modifiedDialogs(response)
		if err != nil {
			return err
		}
		users := tg.UserClassArray(page.GetUsers()).UserToMap()
		collect(page.GetDialogs(), users, page.GetMessages())
		if _, complete := response.(*tg.MessagesDialogs); complete || len(page.GetDialogs()) == 0 {
			return nil
		}
		cursor, offsetPeer, err := nextDialogCursor(page, users, r.selfID)
		if err != nil {
			return err
		}
		if _, exists := seen[cursor]; exists {
			return errors.New("Telegram dialog pagination did not advance")
		}
		seen[cursor] = struct{}{}
		request.OffsetDate = cursor.date
		request.OffsetID = cursor.messageID
		request.OffsetPeer = offsetPeer
	}
}

type dialogCursor struct {
	peer      telegramPeerKey
	messageID int
	date      int
}

func nextDialogCursor(page tg.ModifiedMessagesDialogs, users map[int64]*tg.User, selfID int64) (dialogCursor, tg.InputPeerClass, error) {
	dialogs := page.GetDialogs()
	for i := len(dialogs) - 1; i >= 0; i-- {
		dialog, ok := dialogs[i].(*tg.Dialog)
		if !ok {
			continue
		}
		key := dialogPeerKey(dialog.GetPeer())
		if key.id <= 0 || dialog.GetTopMessage() <= 0 {
			continue
		}
		for _, entry := range page.GetMessages() {
			msg, ok := entry.AsNotEmpty()
			if !ok || msg.GetID() != dialog.GetTopMessage() || dialogPeerKey(msg.GetPeerID()) != key {
				continue
			}
			if offsetPeer := cursorPeer(key, page.GetChats(), users, selfID); offsetPeer != nil && msg.GetDate() > 0 {
				return dialogCursor{peer: key, messageID: msg.GetID(), date: msg.GetDate()}, offsetPeer, nil
			}
			break
		}
		continue
	}
	return dialogCursor{}, nil, errors.New("cannot determine the next Telegram dialog page")
}

func cursorPeer(key telegramPeerKey, chats []tg.ChatClass, users map[int64]*tg.User, selfID int64) tg.InputPeerClass {
	switch key.kind {
	case "user":
		if key.id == selfID {
			return &tg.InputPeerSelf{}
		}
		if user := users[key.id]; user != nil && !user.Min {
			if hash, ok := user.GetAccessHash(); ok {
				return &tg.InputPeerUser{UserID: key.id, AccessHash: hash}
			}
		}
	case "group":
		return &tg.InputPeerChat{ChatID: key.id}
	case "channel":
		for _, chat := range chats {
			switch c := chat.(type) {
			case *tg.Channel:
				if hash, ok := c.GetAccessHash(); c.ID == key.id && !c.Min && ok {
					return &tg.InputPeerChannel{ChannelID: key.id, AccessHash: hash}
				}
			case *tg.ChannelForbidden:
				if c.ID == key.id {
					return &tg.InputPeerChannel{ChannelID: key.id, AccessHash: c.AccessHash}
				}
			}
		}
	}
	return nil
}

func (r *accountReader) ListHistory(ctx context.Context, dialog usecase.TelegramDialog, afterID int64) ([]domain.Message, error) {
	if afterID < 0 || afterID > math.MaxInt32 {
		return nil, fmt.Errorf("%w: Telegram history offset exceeds the message ID range", domain.ErrValidation)
	}
	peer := r.peers[dialog.TelegramUserID]
	if peer == nil {
		return nil, fmt.Errorf("%w: load Telegram dialogs before requesting history for this user", domain.ErrValidation)
	}
	request := tg.MessagesGetHistoryRequest{Peer: peer, Limit: r.limit, MinID: int(afterID)}
	var cutoffUnix int64
	if afterID == 0 && r.initialDays > 0 {
		nowFn := r.now
		if nowFn == nil {
			nowFn = time.Now
		}
		cutoffUnix = nowFn().UTC().AddDate(0, 0, -r.initialDays).Unix()
		if topDate := r.topDates[dialog.TelegramUserID]; topDate > 0 && topDate < cutoffUnix {
			return make([]domain.Message, 0), nil
		}
	}
	result := make([]domain.Message, 0)
	seen := make(map[int]struct{})
	for {
		response, err := retryTelegramRPC(ctx, func(ctx context.Context) (tg.MessagesMessagesClass, error) {
			return r.api.MessagesGetHistory(ctx, &request)
		})
		if err != nil {
			return nil, fmt.Errorf("read Telegram history: %w", err)
		}
		if response == nil {
			return nil, errors.New("Telegram returned no history response")
		}
		page, ok := response.AsModified()
		if !ok {
			return nil, fmt.Errorf("unexpected Telegram history response %T", response)
		}
		var oldest int
		hitCutoff := false
		for _, entry := range page.GetMessages() {
			id := entry.GetID()
			if id <= 0 || (request.OffsetID != 0 && id >= request.OffsetID) {
				continue
			}
			if oldest == 0 || id < oldest {
				oldest = id
			}
			message, ok := entry.(*tg.Message)
			if !ok || int64(id) <= afterID {
				continue
			}
			if cutoffUnix > 0 && int64(message.Date) < cutoffUnix {
				hitCutoff = true
				continue
			}
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				result = append(result, mapTelegramMessage(message))
			}
		}
		if len(page.GetMessages()) == 0 {
			break
		}
		if oldest == 0 {
			return nil, errors.New("Telegram history pagination did not advance")
		}
		if _, complete := response.(*tg.MessagesMessages); complete || int64(oldest) <= afterID || hitCutoff {
			break
		}
		request.OffsetID = oldest
	}
	// Telegram returns newest first. Publish only the complete result so the
	// importer can advance its checkpoint without skipping older messages.
	slices.SortFunc(result, func(a, b domain.Message) int { return cmp.Compare(a.TelegramMessageID, b.TelegramMessageID) })
	return result, nil
}

func mapTelegramMessage(msg *tg.Message) domain.Message {
	direction := domain.MessageDirectionIn
	if msg.Out {
		direction = domain.MessageDirectionOut
	}
	return domain.Message{
		TelegramMessageID: int64(msg.ID), Direction: direction, Text: msg.Message,
		SentAt: time.Unix(int64(msg.Date), 0).UTC(), MediaType: telegramMediaType(msg.Media),
	}
}

func telegramMediaType(media tg.MessageMediaClass) *domain.MediaType {
	switch m := media.(type) {
	case *tg.MessageMediaPhoto:
		kind := domain.MediaTypePhoto
		return &kind
	case *tg.MessageMediaDocument:
		kind := domain.MediaTypeDocument
		if m.Voice {
			kind = domain.MediaTypeVoice
		} else if doc, ok := m.Document.(*tg.Document); ok {
			for _, attr := range doc.Attributes {
				if audio, ok := attr.(*tg.DocumentAttributeAudio); ok && audio.Voice {
					kind = domain.MediaTypeVoice
					break
				}
			}
		}
		return &kind
	default:
		return nil
	}
}

func telegramUserName(user *tg.User) string {
	if name := strings.TrimSpace(user.FirstName + " " + user.LastName); name != "" {
		return name
	}
	if user.Username != "" {
		return user.Username
	}
	return fmt.Sprintf("ID %d", user.ID)
}
