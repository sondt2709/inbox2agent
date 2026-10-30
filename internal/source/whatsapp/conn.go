package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// DeviceName is the name of i2a in the list of linked devices on the phone.
const DeviceName = "i2a"

func init() {
	store.SetOSInfo(DeviceName, [3]uint32{1, 0, 0})
}

// ErrBusy is returned when another process holds the connection of the source.
var ErrBusy = errors.New("another i2a process holds the WhatsApp connection (the daemon?)")

// ErrNotPaired is returned when the file of the source has no device keys.
var ErrNotPaired = errors.New("not paired: pair again in i2a config")

// Conn is the open file and the WhatsApp client of one source. One process at
// a time holds it; Open locks LockPath.
type Conn struct {
	id       string
	db       *DB
	cli      *whatsmeow.Client
	unlock   func()
	lookback time.Duration
	log      *slog.Logger
	now      func() time.Time

	offline     chan struct{} // closed at the end of the offline messages
	offlineOnce sync.Once
	chats       chan struct{} // closed after the first read of the groups and contacts
	chatsOnce   sync.Once
	history     chan struct{} // closed at the first history sync
	historyOnce sync.Once

	mu     sync.Mutex
	groups map[string]bool // groups with a name in wa_chats
}

// Open locks the source, opens its file and makes the client. It does not
// connect. It returns ErrBusy when another process holds the source.
func Open(ctx context.Context, dataDir, id string, lookback time.Duration, log *slog.Logger) (*Conn, error) {
	return open(ctx, dataDir, id, lookback, log, 0)
}

func open(ctx context.Context, dataDir, id string, lookback time.Duration, log *slog.Logger, wait time.Duration) (*Conn, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	unlock, err := lockFile(LockPath(dataDir, id), wait)
	if err != nil {
		return nil, err
	}
	wl := slogLogger{log: log.With("source", id)}
	db, err := OpenDB(ctx, Path(dataDir, id), wl)
	if err != nil {
		unlock()
		return nil, err
	}
	dev, err := db.Container.GetFirstDevice(ctx)
	if err != nil {
		db.Close()
		unlock()
		return nil, err
	}
	c := &Conn{id: id, db: db, unlock: unlock, lookback: lookback, log: log, now: time.Now,
		offline: make(chan struct{}), chats: make(chan struct{}), history: make(chan struct{}), groups: map[string]bool{}}
	c.cli = whatsmeow.NewClient(dev, wl.Sub("client"))
	c.cli.AddEventHandler(c.handle)
	return c, nil
}

// DB returns the file of the source.
func (c *Conn) DB() *DB { return c.db }

// Paired reports whether the client has device keys.
func (c *Conn) Paired() bool { return c.cli.Store.ID != nil }

// Connect connects a paired client. whatsmeow reconnects after a network error.
func (c *Conn) Connect(ctx context.Context) error {
	if !c.Paired() {
		return ErrNotPaired
	}
	if c.cli.IsConnected() {
		return nil
	}
	return c.cli.ConnectContext(ctx)
}

// WaitOffline waits until the server sent the messages that arrived while
// the client was offline, or until the time limit.
func (c *Conn) WaitOffline(ctx context.Context, limit time.Duration) {
	select {
	case <-c.offline:
	case <-time.After(limit):
	case <-ctx.Done():
	}
}

// Close disconnects, closes the file and releases the lock.
func (c *Conn) Close() error {
	c.cli.Disconnect()
	err := c.db.Close()
	c.unlock()
	return err
}

// Logout unlinks the device on the phone and deletes its keys. The client
// must be paired; Logout connects when necessary.
func (c *Conn) Logout(ctx context.Context) error {
	if !c.Paired() {
		return nil
	}
	if !c.cli.IsConnected() {
		if err := c.cli.ConnectContext(ctx); err != nil {
			return err
		}
	}
	return c.cli.Logout(ctx)
}

// me returns the addresses of the account.
func (c *Conn) me() Me {
	var me Me
	if id := c.cli.Store.ID; id != nil {
		me.Phone = id.User
	}
	me.LID = c.cli.Store.LID.User
	return me
}

func (c *Conn) handle(evt any) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch v := evt.(type) {
	case *events.Message:
		c.save(ctx, v)
	case *events.HistorySync:
		c.saveHistory(ctx, v)
		c.historyOnce.Do(func() { close(c.history) })
	case *events.Connected:
		go c.readChats()
	case *events.OfflineSyncCompleted:
		c.offlineOnce.Do(func() { close(c.offline) })
	case *events.LoggedOut:
		c.log.Error("whatsapp: the phone unlinked i2a", "source", c.id, "reason", v.Reason.String())
	case *events.StreamReplaced:
		c.log.Error("whatsapp: another client connected with the same keys, so i2a is disconnected", "source", c.id)
	case *events.TemporaryBan:
		c.log.Error("whatsapp: temporary ban", "source", c.id, "ban", v.String())
	}
}

func (c *Conn) save(ctx context.Context, v *events.Message) {
	r, ok := rowFromEvent(ctx, v, c.me(), c)
	if !ok || r.Time.Before(c.now().Add(-c.lookback)) {
		return
	}
	if err := c.db.SaveMessage(ctx, r); err != nil {
		c.log.Error("whatsapp: cannot store a message", "source", c.id, "err", err)
		return
	}
	chat := Chat{JID: r.Chat, Kind: r.Kind, Last: r.Time}
	if r.Kind == inbox.KindDirect {
		chat.Name = c.Name(ctx, v.Info.Chat)
		if chat.Name == "" && !r.FromMe {
			chat.Name = r.SenderName
		}
	}
	if err := c.db.SaveChat(ctx, chat); err != nil {
		c.log.Error("whatsapp: cannot store a chat", "source", c.id, "err", err)
	}
	if r.Kind == inbox.KindGroup {
		c.mu.Lock()
		known := c.groups[r.Chat]
		c.groups[r.Chat] = true
		c.mu.Unlock()
		if !known {
			go c.readGroup(v.Info.Chat)
		}
	}
}

func (c *Conn) saveHistory(ctx context.Context, v *events.HistorySync) {
	for _, conv := range v.Data.GetConversations() {
		jid, err := types.ParseJID(conv.GetID())
		if err != nil {
			continue
		}
		kind, ok := chatKind(jid)
		if !ok {
			continue
		}
		chat := Chat{JID: jid.String(), Kind: kind, Name: conv.GetName()}
		if ts := conv.GetConversationTimestamp(); ts > 0 {
			chat.Last = time.Unix(int64(ts), 0)
		}
		if kind == inbox.KindDirect && chat.Name == "" {
			chat.Name = c.Name(ctx, jid)
		}
		if err := c.db.SaveChat(ctx, chat); err != nil {
			c.log.Error("whatsapp: cannot store a chat", "source", c.id, "err", err)
		}
		for _, hm := range conv.GetMessages() {
			if evt, err := c.cli.ParseWebMessage(jid, hm.GetMessage()); err == nil {
				c.save(ctx, evt)
			}
		}
	}
}

// readChats stores the groups and the named contacts for the picker.
func (c *Conn) readChats() {
	defer c.chatsOnce.Do(func() { close(c.chats) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	groups, err := c.cli.GetJoinedGroups(ctx)
	if err != nil {
		c.log.Warn("whatsapp: cannot read the groups", "source", c.id, "err", err)
	}
	for _, g := range groups {
		if err := c.db.SaveChat(ctx, Chat{JID: g.JID.String(), Name: g.Name, Kind: inbox.KindGroup}); err == nil {
			c.mu.Lock()
			c.groups[g.JID.String()] = true
			c.mu.Unlock()
		}
	}
	contacts, err := c.cli.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		c.log.Warn("whatsapp: cannot read the contacts", "source", c.id, "err", err)
	}
	for jid, ci := range contacts {
		name := contactName(ci, false)
		if name == "" || jid.Server != types.DefaultUserServer {
			continue
		}
		_ = c.db.SaveChat(ctx, Chat{JID: jid.String(), Name: name, Kind: inbox.KindDirect})
	}
}

func (c *Conn) readGroup(jid types.JID) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	g, err := c.cli.GetGroupInfo(ctx, jid)
	if err != nil {
		c.mu.Lock()
		delete(c.groups, jid.String())
		c.mu.Unlock()
		return
	}
	_ = c.db.SaveChat(ctx, Chat{JID: jid.String(), Name: g.Name, Kind: inbox.KindGroup})
}

// phoneJID returns the phone-number address of a LID address when the
// mapping is known, else the address with no device.
func (c *Conn) phoneJID(ctx context.Context, j types.JID) types.JID {
	j = j.ToNonAD()
	if j.Server == types.HiddenUserServer {
		if pn, err := c.cli.Store.LIDs.GetPNForLID(ctx, j); err == nil && !pn.IsEmpty() {
			return pn.ToNonAD()
		}
	}
	return j
}

// Name returns the contact name of an address, or "".
func (c *Conn) Name(ctx context.Context, j types.JID) string {
	for _, k := range []types.JID{c.phoneJID(ctx, j), j.ToNonAD()} {
		if ci, err := c.cli.Store.Contacts.GetContact(ctx, k); err == nil {
			if n := contactName(ci, true); n != "" {
				return n
			}
		}
	}
	return ""
}

// Phone returns the phone number of an address, or "".
func (c *Conn) Phone(ctx context.Context, j types.JID) string {
	if pn := c.phoneJID(ctx, j); pn.Server == types.DefaultUserServer {
		return pn.User
	}
	return ""
}

func contactName(ci types.ContactInfo, push bool) string {
	for _, n := range []string{ci.FullName, ci.FirstName, ci.BusinessName} {
		if n != "" {
			return n
		}
	}
	if push {
		return ci.PushName
	}
	return ""
}

// names finds the contact name and the phone number of an address.
type names interface {
	Name(ctx context.Context, j types.JID) string
	Phone(ctx context.Context, j types.JID) string
}

// chatKind returns the kind of a chat, and false for a chat that i2a does not
// read: status updates, broadcast lists, newsletters and other servers.
func chatKind(j types.JID) (string, bool) {
	switch j.Server {
	case types.GroupServer:
		return inbox.KindGroup, true
	case types.DefaultUserServer, types.HiddenUserServer:
		return inbox.KindDirect, true
	}
	return "", false
}

// rowFromEvent makes the stored row of a message. It returns false for a
// message that i2a does not read.
func rowFromEvent(ctx context.Context, v *events.Message, me Me, n names) (Row, bool) {
	kind, ok := chatKind(v.Info.Chat)
	if !ok || v.IsEdit || v.Message == nil {
		return Row{}, false
	}
	text := Text(v.Message)
	if text == "" {
		return Row{}, false
	}
	raw, err := proto.Marshal(v.Message)
	if err != nil {
		return Row{}, false
	}
	r := Row{Chat: v.Info.Chat.String(), ID: v.Info.ID, Sender: v.Info.Sender.ToNonAD().String(), Kind: kind,
		FromMe: v.Info.IsFromMe, Time: v.Info.Timestamp, Text: text, Raw: raw}
	who := v.Info.Sender
	if who.Server == types.HiddenUserServer && v.Info.SenderAlt.Server == types.DefaultUserServer {
		who = v.Info.SenderAlt
	}
	if !r.FromMe {
		r.SenderName = n.Name(ctx, who)
		if r.SenderName == "" {
			r.SenderName = v.Info.PushName
		}
		if r.SenderName == "" {
			if p := n.Phone(ctx, who); p != "" {
				r.SenderName = "+" + p
			}
		}
	}
	if kind == inbox.KindGroup {
		r.Mention = !r.FromMe && MentionsMe(v.Message, me)
	} else {
		chat := v.Info.Chat
		switch {
		case chat.Server == types.DefaultUserServer:
			r.Phone = chat.User
		case !r.FromMe && v.Info.SenderAlt.Server == types.DefaultUserServer:
			r.Phone = v.Info.SenderAlt.User
		case r.FromMe && v.Info.RecipientAlt.Server == types.DefaultUserServer:
			r.Phone = v.Info.RecipientAlt.User
		default:
			r.Phone = n.Phone(ctx, chat)
		}
	}
	return r, true
}

// lockFile takes an exclusive lock. With a wait above 0, it tries again until
// the wait ends.
func lockFile(path string, wait time.Duration) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	end := time.Now().Add(wait)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		if time.Now().After(end) {
			f.Close()
			return nil, ErrBusy
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// slogLogger passes the warnings and errors of whatsmeow to the i2a log.
type slogLogger struct {
	log    *slog.Logger
	module string
}

// noise are the starts of whatsmeow warnings that are not problems: a
// disconnect always writes "Error sending close to websocket".
var noise = []string{"Error sending close to websocket"}

func (l slogLogger) Warnf(msg string, args ...any) {
	text := fmt.Sprintf(msg, args...)
	for _, n := range noise {
		if strings.HasPrefix(text, n) {
			return
		}
	}
	l.log.Warn("whatsmeow: "+text, "module", l.module)
}

func (l slogLogger) Errorf(msg string, args ...any) {
	l.log.Error("whatsmeow: "+fmt.Sprintf(msg, args...), "module", l.module)
}
func (l slogLogger) Infof(string, ...any)  {}
func (l slogLogger) Debugf(string, ...any) {}
func (l slogLogger) Sub(module string) waLog.Logger {
	if l.module != "" {
		module = l.module + "/" + module
	}
	return slogLogger{log: l.log, module: module}
}
