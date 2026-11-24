package linear

import (
	"regexp"
	"slices"
	"strings"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// mentionRe matches one leading mention: @<name> or a Linear profile URL,
// with an optional comma or colon after it.
var mentionRe = regexp.MustCompile(`^(?:@[\p{L}\p{N}._-]+|https://linear\.app/[^/\s]+/profiles/[^\s,:]+)[,:]?(?:\s+|$)`)

// afterMentions returns the body without leading spaces and leading mentions.
func afterMentions(body string) string {
	rest := strings.TrimLeft(body, " \t\r\n")
	for {
		loc := mentionRe.FindStringIndex(rest)
		if loc == nil {
			return rest
		}
		rest = rest[loc[1]:]
	}
}

// hasPrefix reports whether the body starts with the prefix after the leading
// mentions. One Markdown backslash before the prefix is ignored, because the
// Linear editor can escape a character such as > at the start of a line.
func hasPrefix(body, prefix string) bool {
	rest := afterMentions(body)
	return strings.HasPrefix(rest, prefix) || strings.HasPrefix(rest, `\`) && strings.HasPrefix(rest[1:], prefix)
}

// isFrom reports whether the user is in the list: "me" is the user of the
// key, another value is a display name, a name or an email, without case.
func isFrom(u *User, from []string) bool {
	if u == nil {
		return false
	}
	for _, f := range from {
		if f == config.FromMe && u.IsMe {
			return true
		}
		if strings.EqualFold(f, u.DisplayName) || strings.EqualFold(f, u.Email) || strings.EqualFold(f, u.Name) {
			return true
		}
	}
	return false
}

func containsFold(list []string, v string) bool {
	return slices.ContainsFunc(list, func(s string) bool { return strings.EqualFold(s, v) })
}

// issueMatches applies the project, label and team lists of a feed.
func issueMatches(f config.Feed, is *Issue) bool {
	if len(f.Projects) == 0 && len(f.Labels) == 0 && len(f.Teams) == 0 {
		return true
	}
	if is == nil {
		return false
	}
	if len(f.Projects) > 0 && (is.Project == nil || !containsFold(f.Projects, is.Project.Name)) {
		return false
	}
	if len(f.Teams) > 0 && (is.Team == nil || !containsFold(f.Teams, is.Team.Key)) {
		return false
	}
	if len(f.Labels) > 0 && !slices.ContainsFunc(is.Labels.Nodes, func(l Label) bool { return containsFold(f.Labels, l.Name) }) {
		return false
	}
	return true
}

// commentMatches applies the rule of a comments feed to a comment.
func commentMatches(f config.Feed, c Comment) bool {
	from := f.From
	if len(from) == 0 {
		from = []string{config.FromMe}
	}
	if c.Issue == nil || !isFrom(c.User, from) || !issueMatches(f, c.Issue) {
		return false
	}
	return f.Prefix == "" || hasPrefix(c.Body, f.Prefix)
}

// noteMatches applies the rule of an inbox feed to a notification. An empty
// From means anyone.
func noteMatches(f config.Feed, n Notification) bool {
	if len(f.From) > 0 && !isFrom(n.Actor, f.From) {
		return false
	}
	if !issueMatches(f, n.Issue) {
		return false
	}
	if f.Prefix == "" {
		return true
	}
	return n.Comment != nil && hasPrefix(n.Comment.Body, f.Prefix)
}
