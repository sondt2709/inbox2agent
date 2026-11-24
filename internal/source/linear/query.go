package linear

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Viewer is the user of the API key and the workspace.
type Viewer struct {
	ID           string `json:"id"`
	DisplayName  string `json:"displayName"`
	Organization struct {
		Name   string `json:"name"`
		URLKey string `json:"urlKey"`
	} `json:"organization"`
}

// ViewerQuery reads the user of the key and the workspace.
const ViewerQuery = `query Viewer { viewer { id displayName organization { name urlKey } } }`

// User is the author of a comment or the actor of a notification.
type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	IsMe        bool   `json:"isMe"`
}

// Issue is the issue of a comment or of a notification.
type Issue struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Team       *struct {
		Key string `json:"key"`
	} `json:"team"`
	Project *struct {
		Name string `json:"name"`
	} `json:"project"`
	Labels struct {
		Nodes []Label `json:"nodes"`
	} `json:"labels"`
}

// Label is a label of an issue.
type Label struct {
	Name string `json:"name"`
}

// Parent is the comment that a comment replies to.
type Parent struct {
	ID   string `json:"id"`
	Body string `json:"body"`
	User *User  `json:"user"`
}

// Comment is an issue comment.
type Comment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	User      *User     `json:"user"`
	Parent    *Parent   `json:"parent"`
	Issue     *Issue    `json:"issue"`
}

// Notification is one item of the Linear inbox of the user.
type Notification struct {
	Typename  string    `json:"__typename"`
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"createdAt"`
	Actor     *User     `json:"actor"`
	Issue     *Issue    `json:"issue"`   // IssueNotification only
	Comment   *Comment  `json:"comment"` // IssueNotification only
}

// PageInfo tells if a connection has more nodes.
type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// Page is one page of a connection.
type Page struct {
	Nodes    json.RawMessage `json:"nodes"`
	PageInfo PageInfo        `json:"pageInfo"`
}

// Part is the request of one feed in a poll document.
type Part struct {
	Kind   string    // config.FeedComments or config.FeedInbox
	Since  time.Time // comments: updatedAt after it; inbox: createdAt after it
	After  string    // the end cursor of the last page; "" for the first page
	Prefix string    // comments: the body must contain it; "" for no rule
	MeOnly bool      // comments: only the comments of the user of the key
}

const (
	fragUser    = `fragment U on User { id displayName name email isMe }`
	fragIssue   = `fragment I on Issue { id identifier title url team { key } project { name } labels(first: 20) { nodes { name } } }`
	fragComment = `fragment C on Comment { id body url createdAt updatedAt user { ...U } parent { id body user { ...U } } issue { ...I } }`
	fragNote    = `fragment N on Notification { __typename id type createdAt actor { ...U } ... on IssueNotification { issue { ...I } comment { ...C } } }`
)

// Alias returns the response field of part i.
func Alias(i int) string { return fmt.Sprintf("f%d", i) }

// PollQuery returns one query document for the parts, with one alias for
// each part (Alias), and its variables. first is the page size.
func PollQuery(parts []Part, first int) (string, map[string]any) {
	var params, fields []string
	vars := map[string]any{}
	notes := false
	for i, p := range parts {
		t, a := fmt.Sprintf("t%d", i), fmt.Sprintf("a%d", i)
		params = append(params, "$"+t+": DateTimeOrDuration!", "$"+a+": String")
		vars[t] = p.Since.UTC().Format(time.RFC3339Nano)
		if p.After != "" {
			vars[a] = p.After
		}
		page := "{ nodes { ...%s } pageInfo { hasNextPage endCursor } }"
		if p.Kind == "inbox" {
			notes = true
			fields = append(fields, fmt.Sprintf("%s: notifications(first: %d, after: $%s, orderBy: createdAt, filter: { createdAt: { gt: $%s } }) "+page,
				Alias(i), first, a, t, "N"))
			continue
		}
		filter := []string{"updatedAt: { gt: $" + t + " }"}
		if p.MeOnly {
			filter = append(filter, "user: { isMe: { eq: true } }")
		}
		if p.Prefix != "" {
			pv := fmt.Sprintf("p%d", i)
			params = append(params, "$"+pv+": String!")
			vars[pv] = p.Prefix
			filter = append(filter, "body: { contains: $"+pv+" }")
		}
		fields = append(fields, fmt.Sprintf("%s: comments(first: %d, after: $%s, orderBy: updatedAt, filter: { %s }) "+page,
			Alias(i), first, a, strings.Join(filter, ", "), "C"))
	}
	frags := []string{fragUser, fragIssue, fragComment}
	if notes {
		frags = append(frags, fragNote)
	}
	doc := "query Poll(" + strings.Join(params, ", ") + ") {\n  " + strings.Join(fields, "\n  ") + "\n}\n" + strings.Join(frags, "\n")
	return doc, vars
}
