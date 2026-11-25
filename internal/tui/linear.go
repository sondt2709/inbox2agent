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

// linearUser splits config.Account.User of Linear (<display name>@<workspace key>).
func linearUser(user string) (name, workspace string) {
	if i := strings.LastIndex(user, "@"); i > 0 {
		return user[:i], user[i+1:]
	}
	return user, ""
}
