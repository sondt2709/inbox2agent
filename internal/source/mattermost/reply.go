package mattermost

import (
	"context"
	"strings"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// Reply sends a reply to a stored message as the user of the token. It reads
// the target post first: the post gives the channel and the thread root, and
// the request fails when the post was deleted. Mattermost needs the root of the
// thread in root_id; the ID of a reply in the thread gives HTTP 400.
func (src *Source) Reply(ctx context.Context, r inbox.ReplyRequest) (inbox.Posted, error) {
	me, err := src.whoAmI(ctx)
	if err != nil {
		return inbox.Posted{}, err
	}
	target, err := src.client.GetPost(ctx, r.Target.ExternalID)
	if err != nil {
		return inbox.Posted{}, err
	}
	// The web app uses "<user id>:<time>" too. The server returns the first
	// post again for a known pending_post_id, for a limited time.
	p, err := src.client.CreatePost(ctx, NewPost{
		ChannelID: target.ChannelID, Message: r.Text, RootID: replyRoot(target, r.Thread),
		PendingPostID: me.ID + ":" + r.PendingID,
	})
	if err != nil {
		return inbox.Posted{}, err
	}
	return inbox.Posted{ID: p.ID, URL: permalink(r.Target.URL, p.ID)}, nil
}

// Find looks for a post of the user with the text of the reply, in the place
// of the reply, created after the first try. It reads the newest page of the
// channel, so it finds a post of the last 200 posts only.
func (src *Source) Find(ctx context.Context, r inbox.ReplyRequest) (inbox.Posted, bool, error) {
	me, err := src.whoAmI(ctx)
	if err != nil {
		return inbox.Posted{}, false, err
	}
	target, err := src.client.GetPost(ctx, r.Target.ExternalID)
	if err != nil {
		return inbox.Posted{}, false, err
	}
	root := replyRoot(target, r.Thread)
	pl, err := src.client.PostsPage(ctx, target.ChannelID, 0, 200)
	if err != nil {
		return inbox.Posted{}, false, err
	}
	for _, id := range pl.Order {
		p, ok := pl.Posts[id]
		if ok && p.UserID == me.ID && p.DeleteAt == 0 && p.Message == r.Text && p.RootID == root &&
			p.CreateAt >= r.Since.UnixMilli() {
			return inbox.Posted{ID: p.ID, URL: permalink(r.Target.URL, p.ID)}, true, nil
		}
	}
	return inbox.Posted{}, false, nil
}

// replyRoot returns the root_id of a reply to the post: the root of its
// thread, or the post itself when it is a top-level post. A reply in the
// channel has no root.
func replyRoot(target Post, thread bool) string {
	switch {
	case !thread:
		return ""
	case target.RootID != "":
		return target.RootID
	}
	return target.ID
}

// permalink makes the URL of a new post from the URL of the target post
// ("<server>/<team>/pl/<id>").
func permalink(targetURL, id string) string {
	i := strings.LastIndex(targetURL, "/pl/")
	if i < 0 {
		return ""
	}
	return targetURL[:i] + "/pl/" + id
}
