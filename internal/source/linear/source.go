package linear

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// SourceType is the type name in the configuration.
const SourceType = config.PlatformLinear

const (
	// maxPages limits the pages of one feed in one poll.
	maxPages = 10
	// pageSize is the number of items on one page.
	pageSize = 50
	// overlap is the time before the cursor that each request reads again:
	// Linear can give an item a time before a time that a poll saw. The store
	// drops the items that it has.
	overlap = 2 * time.Minute
	// quoteMax limits the parent text in a reply reference.
	quoteMax = 200
)

// Settings are the values that the source needs from the configuration.
type Settings struct {
	ID       string
	Feeds    []config.Feed
	Levels   map[string]string // the read level of each feed that has an entry
	Default  string            // the read level of the other feeds (unsorted.read)
	Lookback time.Duration
	PageSize int // 0 means pageSize
	MaxPages int // 0 means maxPages
	// CatalogPage is the number of names on one page of Catalog; 0 means catalogPage.
	CatalogPage int
	Log         *slog.Logger // nil means no log
}

// Source collects the comments and the notifications of the feeds of one
// Linear account. It sends only queries.
type Source struct {
	s      Settings
	client *Client
}

// New returns a source.
func New(s Settings, client *Client) *Source {
	if s.PageSize <= 0 {
		s.PageSize = pageSize
	}
	if s.MaxPages <= 0 {
		s.MaxPages = maxPages
	}
	if s.CatalogPage <= 0 {
		s.CatalogPage = catalogPage
	}
	return &Source{s: s, client: client}
}

// ID returns the account ID.
func (src *Source) ID() string { return src.s.ID }

// Check reads the user of the key and the workspace.
func (src *Source) Check(ctx context.Context) (Viewer, error) {
	var out struct {
		Viewer Viewer `json:"viewer"`
	}
	if err := src.client.Query(ctx, ViewerQuery, nil, &out); err != nil {
		return Viewer{}, err
	}
	return out.Viewer, nil
}

// User returns the value of config.Account.User: <display name>@<workspace key>.
func (v Viewer) User() string {
	if v.Organization.URLKey == "" {
		return v.DisplayName
	}
	return v.DisplayName + "@" + v.Organization.URLKey
}

func (src *Source) level(id string) string {
	if l, ok := src.s.Levels[id]; ok {
		return l
	}
	return src.s.Default
}

// polled is one feed of a poll and its state.
type polled struct {
	feed  config.Feed
	all   bool // the read level is all
	part  Part
	since time.Time // the cursor time of the feed
	next  time.Time // the newest time seen
	done  bool
}

// Collect returns the matching items after the cursor of each feed, and the
// new cursor: one RFC 3339 time for each feed.
func (src *Source) Collect(ctx context.Context, cur inbox.Cursor, now time.Time) ([]inbox.Message, inbox.Cursor, error) {
	floor := now.Add(-src.s.Lookback)
	var feeds []*polled
	for _, f := range src.s.Feeds {
		var all bool
		switch src.level(f.ID) {
		case config.ReadAll:
			all = true
		case config.ReadMentions:
		default:
			continue
		}
		since := floor
		if t, err := time.Parse(time.RFC3339Nano, cur[f.ID]); err == nil && t.After(since) {
			since = t
		}
		p := &polled{feed: f, all: all, since: since, next: since}
		p.part = Part{Kind: f.Kind, Since: since.Add(-overlap)}
		if f.Kind == config.FeedComments {
			p.part.Prefix = f.Prefix
			p.part.MeOnly = len(f.From) == 0 || slices.Equal(f.From, []string{config.FromMe})
		}
		feeds = append(feeds, p)
	}
	var msgs []inbox.Message
	for page := 0; page < src.s.MaxPages; page++ {
		var parts []Part
		var open []*polled
		for _, p := range feeds {
			if !p.done {
				parts, open = append(parts, p.part), append(open, p)
			}
		}
		if len(parts) == 0 {
			break
		}
		doc, vars := PollQuery(parts, src.s.PageSize)
		var data map[string]Page
		if err := src.client.Query(ctx, doc, vars, &data); err != nil {
			return nil, nil, fmt.Errorf("linear account %s: %w", src.s.ID, err)
		}
		for i, p := range open {
			pg := data[Alias(i)]
			got, err := src.messages(p, pg.Nodes)
			if err != nil {
				return nil, nil, fmt.Errorf("linear account %s, feed %s: %w", src.s.ID, p.feed.ID, err)
			}
			msgs = append(msgs, got...)
			p.done = !pg.PageInfo.HasNextPage
			p.part.After = pg.PageInfo.EndCursor
		}
	}
	next := inbox.Cursor{}
	for _, p := range feeds {
		if !p.done && src.s.Log != nil {
			src.s.Log.Warn("linear: a feed has more items than the page limit; the older items are skipped",
				"account", src.s.ID, "feed", p.feed.ID, "pages", src.s.MaxPages)
		}
		next[p.feed.ID] = p.next.UTC().Format(time.RFC3339Nano)
	}
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].CreatedAt.Before(msgs[j].CreatedAt) })
	return msgs, next, nil
}

// messages decodes one page of a feed, moves the newest time of the feed and
// returns the matching items as messages.
func (src *Source) messages(p *polled, nodes json.RawMessage) ([]inbox.Message, error) {
	var out []inbox.Message
	if p.feed.Kind == config.FeedInbox {
		var notes []Notification
		if err := json.Unmarshal(nodes, &notes); err != nil {
			return nil, err
		}
		for _, n := range notes {
			if n.CreatedAt.After(p.next) {
				p.next = n.CreatedAt
			}
			forMe := noteForMe(n.Type)
			if !noteMatches(p.feed, n) || !p.all && !forMe {
				continue
			}
			out = append(out, src.noteMessage(p.feed, n, forMe))
		}
		return out, nil
	}
	var comments []Comment
	if err := json.Unmarshal(nodes, &comments); err != nil {
		return nil, err
	}
	for _, c := range comments {
		if c.UpdatedAt.After(p.next) {
			p.next = c.UpdatedAt
		}
		if !commentMatches(p.feed, c) {
			continue
		}
		m := src.base(p.feed, c.Issue)
		m.ExternalID, m.Author, m.URL, m.CreatedAt, m.ForMe = c.ID, name(c.User), orStr(c.URL, c.Issue.URL), c.CreatedAt.UTC(), true
		m.Text = heading(c.Issue) + "\n\n" + c.Body
		m.ReplyTo = replyRef(c.Parent)
		out = append(out, m)
	}
	return out, nil
}

func (src *Source) noteMessage(f config.Feed, n Notification, forMe bool) inbox.Message {
	m := src.base(f, n.Issue)
	m.ExternalID, m.Author, m.CreatedAt, m.ForMe = n.ID, name(n.Actor), n.CreatedAt.UTC(), forMe
	body := phrase(n.Type)
	if n.Comment != nil {
		body = n.Comment.Body
		m.ReplyTo = replyRef(n.Comment.Parent)
		m.URL = n.Comment.URL
	}
	if n.Issue != nil {
		m.Text = heading(n.Issue) + "\n\n" + body
		m.URL = orStr(m.URL, n.Issue.URL)
	} else {
		m.Text = body
	}
	return m
}

// base returns a message with the fields of the feed and the issue.
func (src *Source) base(f config.Feed, is *Issue) inbox.Message {
	m := inbox.Message{SourceID: src.s.ID, SourceType: SourceType, ChannelID: f.ID, ChannelName: f.Title(), ChannelKind: inbox.KindChannel}
	if is != nil {
		m.ThreadID = is.Identifier
	}
	return m
}

// noteForMe reports whether a notification type is for the user: an
// assignment, a mention or a review request.
func noteForMe(t string) bool {
	return strings.Contains(t, "Mention") || t == "issueAssignedToYou" || t == "pullRequestReviewRequested"
}

// phrase describes a notification with no comment, from its type:
// issueStatusChanged becomes "issue status changed".
func phrase(t string) string {
	var b strings.Builder
	for i, r := range t {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte(' ')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func heading(is *Issue) string { return is.Identifier + " " + is.Title }

func name(u *User) string {
	if u == nil {
		return ""
	}
	return orStr(u.DisplayName, u.Name)
}

func replyRef(p *Parent) *inbox.ReplyRef {
	if p == nil {
		return nil
	}
	ref := &inbox.ReplyRef{Ext: p.ID, Text: snippet(p.Body)}
	if p.User != nil && p.User.IsMe {
		ref.Me = true
	} else {
		ref.Author = name(p.User)
	}
	return ref
}

// snippet returns at most quoteMax characters of a text.
func snippet(s string) string {
	r := []rune(s)
	if len(r) <= quoteMax {
		return s
	}
	return string(r[:quoteMax])
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
