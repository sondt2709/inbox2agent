package mattermost

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// SourceType is the type name in the configuration.
const SourceType = "mattermost"

// maxPages limits the pages read from one channel in one poll.
const maxPages = 50

// Settings are the values that the source needs from the configuration.
type Settings struct {
	ID       string
	URL      string
	Levels   map[string]string // the read level of each channel that has an entry
	Default  string            // the read level of the other channels (unsorted.read)
	Lookback time.Duration
	PerPage  int // 0 means 200
}

// Source collects new posts for one Mattermost user.
type Source struct {
	s      Settings
	client *Client

	mu    sync.Mutex
	me    *User
	users map[string]string // user ID -> username
}

// New returns a source.
func New(s Settings, client *Client) *Source {
	if s.PerPage <= 0 {
		s.PerPage = 200
	}
	return &Source{s: s, client: client, users: map[string]string{}}
}

// ID returns the source ID.
func (src *Source) ID() string { return src.s.ID }

// ChannelInfo describes one channel for the channel picker.
type ChannelInfo struct {
	ID       string
	Team     string // team name; empty for direct and group channels
	Kind     string // inbox.KindChannel, KindDirect or KindGroup
	Private  bool
	Name     string
	Selected bool
	LastPost time.Time // time of the last post; zero when the channel has no post
}

type channelRef struct {
	Channel
	teamName string
	label    string
	kind     string
}

func (src *Source) whoAmI(ctx context.Context) (User, error) {
	src.mu.Lock()
	defer src.mu.Unlock()
	if src.me != nil {
		return *src.me, nil
	}
	u, err := src.client.Me(ctx)
	if err != nil {
		return User{}, err
	}
	src.me = &u
	src.users[u.ID] = u.Username
	return u, nil
}

// Check calls the server with the token and returns the username.
func (src *Source) Check(ctx context.Context) (string, error) {
	u, err := src.whoAmI(ctx)
	return u.Username, err
}

// usernames resolves user IDs to usernames, with a cache.
func (src *Source) usernames(ctx context.Context, ids []string) error {
	src.mu.Lock()
	var missing []string
	seen := map[string]bool{}
	for _, id := range ids {
		if _, ok := src.users[id]; !ok && id != "" && !seen[id] {
			missing = append(missing, id)
			seen[id] = true
		}
	}
	src.mu.Unlock()
	if len(missing) == 0 {
		return nil
	}
	users, err := src.client.UsersByIDs(ctx, missing)
	if err != nil {
		return err
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	for _, u := range users {
		src.users[u.ID] = u.Username
	}
	return nil
}

func (src *Source) username(id string) string {
	src.mu.Lock()
	defer src.mu.Unlock()
	if n, ok := src.users[id]; ok {
		return n
	}
	return id
}

// channels returns the channels of the user in all teams, without duplicates,
// with a readable label for each channel.
func (src *Source) channels(ctx context.Context, me User) ([]channelRef, []Team, error) {
	teams, err := src.client.Teams(ctx)
	if err != nil {
		return nil, nil, err
	}
	byID := map[string]channelRef{}
	var order []string
	teamName := map[string]string{}
	for _, t := range teams {
		teamName[t.ID] = t.Name
	}
	for _, t := range teams {
		chs, err := src.client.Channels(ctx, t.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, ch := range chs {
			if ch.DeleteAt != 0 {
				continue
			}
			if _, ok := byID[ch.ID]; ok {
				continue
			}
			byID[ch.ID] = channelRef{Channel: ch, teamName: teamName[ch.TeamID]}
			order = append(order, ch.ID)
		}
	}
	// Direct channel names are "<user-id>__<user-id>". Resolve the other user.
	var others []string
	for _, id := range order {
		ch := byID[id]
		if ch.Type == "D" {
			others = append(others, otherUser(ch.Name, me.ID))
		}
	}
	if err := src.usernames(ctx, others); err != nil {
		return nil, nil, err
	}
	out := make([]channelRef, 0, len(order))
	for _, id := range order {
		ch := byID[id]
		switch ch.Type {
		case "D":
			ch.kind = inbox.KindDirect
			ch.label = "@" + src.username(otherUser(ch.Name, me.ID))
		case "G":
			ch.kind = inbox.KindGroup
			ch.label = groupLabel(ch.DisplayName, me.Username)
		default:
			ch.kind = inbox.KindChannel
			ch.label = ch.Name
		}
		out = append(out, ch)
	}
	return out, teams, nil
}

// groupLabel removes the user from the display name of a group channel
// ("alice, bob, carol"), so the label names the other people only.
func groupLabel(display, me string) string {
	names := strings.Split(display, ", ")
	others := slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == me })
	if len(others) == 0 {
		return display
	}
	return strings.Join(others, ", ")
}

func otherUser(dmName, me string) string {
	a, b, ok := strings.Cut(dmName, "__")
	if !ok {
		return ""
	}
	if a == me {
		return b
	}
	return a
}

// ListChannels returns the channels of the user for the channel picker.
func (src *Source) ListChannels(ctx context.Context) ([]ChannelInfo, error) {
	me, err := src.whoAmI(ctx)
	if err != nil {
		return nil, err
	}
	chs, _, err := src.channels(ctx, me)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelInfo, 0, len(chs))
	for _, ch := range chs {
		info := ChannelInfo{
			ID: ch.ID, Team: ch.teamName, Kind: ch.kind, Private: ch.Type == "P",
			Name: ch.label, Selected: src.s.Levels[ch.ID] != "",
		}
		if ch.LastPostAt > 0 {
			info.LastPost = time.UnixMilli(ch.LastPostAt)
		}
		out = append(out, info)
	}
	SortChannels(out)
	return out, nil
}

// SortChannels puts the team channels first, by team and name. Then it puts the
// direct and group conversations, the most recent first, because a user has many
// of them and talks to a few people often.
func SortChannels(chs []ChannelInfo) {
	conv := func(c ChannelInfo) bool { return c.Kind != inbox.KindChannel }
	sort.SliceStable(chs, func(i, j int) bool {
		a, b := chs[i], chs[j]
		if conv(a) != conv(b) {
			return !conv(a)
		}
		if conv(a) {
			if !a.LastPost.Equal(b.LastPost) {
				return a.LastPost.After(b.LastPost)
			}
			return a.Name < b.Name
		}
		if a.Team != b.Team {
			return a.Team < b.Team
		}
		return a.Name < b.Name
	})
}

// Collect returns the posts created after the cursor of each candidate channel,
// and the new cursor. It uses only read requests, so the read state does not change.
func (src *Source) Collect(ctx context.Context, cur inbox.Cursor, now time.Time) ([]inbox.Message, inbox.Cursor, error) {
	me, err := src.whoAmI(ctx)
	if err != nil {
		return nil, nil, err
	}
	chs, teams, err := src.channels(ctx, me)
	if err != nil {
		return nil, nil, err
	}
	defaultTeam := ""
	if len(teams) > 0 {
		defaultTeam = teams[0].Name
	}
	mention := mentionMatcher(me.Username)
	floor := now.Add(-src.s.Lookback).UnixMilli()

	var msgs []inbox.Message
	next := inbox.Cursor{}
	for _, ch := range chs {
		// A direct channel has only the user and one other person, so each post
		// in it is for the user: the level mentions reads all of it.
		var all bool
		switch src.level(ch.ID) {
		case config.ReadAll:
			all = true
		case config.ReadMentions:
			all = ch.Type == "D"
		default:
			continue
		}
		since := floor
		if v, ok := cur[ch.ID]; ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > since {
				since = n
			}
		}
		if ch.LastPostAt <= since {
			continue
		}
		posts, newest, err := src.newPosts(ctx, ch.ID, since)
		if err != nil {
			return nil, nil, fmt.Errorf("channel %s: %w", ch.label, err)
		}
		next[ch.ID] = strconv.FormatInt(newest, 10)

		var authors []string
		for _, p := range posts {
			authors = append(authors, p.UserID)
		}
		if err := src.usernames(ctx, authors); err != nil {
			return nil, nil, err
		}
		team := ch.teamName
		if team == "" {
			team = defaultTeam
		}
		for _, p := range posts {
			if p.UserID == me.ID || p.DeleteAt != 0 || p.Type != "" {
				continue
			}
			if !all && !mention.MatchString(p.Message) {
				continue
			}
			msgs = append(msgs, inbox.Message{
				SourceID:    src.s.ID,
				SourceType:  SourceType,
				ExternalID:  p.ID,
				ChannelID:   ch.ID,
				ChannelName: ch.label,
				ChannelKind: ch.kind,
				ThreadID:    p.RootID,
				Author:      src.username(p.UserID),
				Text:        p.Message,
				URL:         strings.TrimRight(src.s.URL, "/") + "/" + team + "/pl/" + p.ID,
				CreatedAt:   time.UnixMilli(p.CreateAt).UTC(),
			})
		}
	}
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].CreatedAt.Before(msgs[j].CreatedAt) })
	return msgs, next, nil
}

// level returns the read level of a channel.
func (src *Source) level(id string) string {
	if l, ok := src.s.Levels[id]; ok {
		return l
	}
	return src.s.Default
}

// newPosts reads pages of a channel, newest first, until it reaches a post
// that is not newer than since. It returns the new posts and the newest create time.
func (src *Source) newPosts(ctx context.Context, channelID string, since int64) ([]Post, int64, error) {
	var out []Post
	newest := since
	for page := 0; page < maxPages; page++ {
		pl, err := src.client.PostsPage(ctx, channelID, page, src.s.PerPage)
		if err != nil {
			return nil, 0, err
		}
		done := len(pl.Order) < src.s.PerPage
		for _, id := range pl.Order {
			p, ok := pl.Posts[id]
			if !ok {
				continue
			}
			if p.CreateAt <= since {
				done = true
				break
			}
			if p.CreateAt > newest {
				newest = p.CreateAt
			}
			out = append(out, p)
		}
		if done {
			break
		}
	}
	return out, newest, nil
}

// mentionMatcher matches "@username" as a whole word. Mattermost usernames can
// contain letters, digits, ".", "_" and "-". A final "." is punctuation.
func mentionMatcher(username string) *regexp.Regexp {
	u := regexp.QuoteMeta(strings.ToLower(username))
	return regexp.MustCompile(`(?i)(?:^|[^a-z0-9_.\-@])@` + u + `(?:$|[^a-z0-9_\-.]|\.(?:$|[^a-z0-9_\-]))`)
}
