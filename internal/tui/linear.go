package tui

import (
	"strings"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/source/mattermost"
)

// feedInfos returns the feeds of a Linear account as its conversations. They
// come from the configuration, so a feed that the user adds to config.toml
// shows at the next refresh.
func feedInfos(a config.Account) []mattermost.ChannelInfo {
	out := make([]mattermost.ChannelInfo, 0, len(a.Feeds))
	for _, f := range a.Feeds {
		out = append(out, mattermost.ChannelInfo{ID: f.ID, Kind: inbox.KindChannel, Name: f.Title()})
	}
	return out
}

// feedLine describes a feed in one line, for example
// "agent · comments · from me · prefix /agent".
func feedLine(f config.Feed) string {
	parts := []string{f.ID, f.Kind}
	from := f.From
	if len(from) == 0 && f.Kind == config.FeedComments {
		from = []string{config.FromMe}
	}
	if len(from) > 0 {
		parts = append(parts, "from "+strings.Join(from, ", "))
	}
	if f.Prefix != "" {
		parts = append(parts, "prefix "+f.Prefix)
	}
	for _, l := range []struct {
		name string
		list []string
	}{{"projects", f.Projects}, {"labels", f.Labels}, {"teams", f.Teams}} {
		if len(l.list) > 0 {
			parts = append(parts, l.name+" "+strings.Join(l.list, ", "))
		}
	}
	return strings.Join(parts, " · ")
}

// linearUser splits config.Account.User of Linear (<display name>@<workspace key>).
func linearUser(user string) (name, workspace string) {
	if i := strings.LastIndex(user, "@"); i > 0 {
		return user[:i], user[i+1:]
	}
	return user, ""
}
