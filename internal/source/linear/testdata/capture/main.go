// Command capture saves real responses of the Linear GraphQL API as test
// fixtures, with the personal data replaced.
//
// Post the test comments in one issue (the -issue flag), then run from the
// repository root:
//
//	go run ./internal/source/linear/testdata/capture -key-file <file> -issue ENG-123
//
// It sends the queries of the source (linear.ViewerQuery, linear.PollQuery
// and linear.CatalogQuery) and writes one file for each case in
// internal/source/linear/testdata: {"status": <HTTP status>, "body": <JSON>}.
// The tool replaces each ID, cursor, name, email, team key, project, label,
// workspace key and URL with a fake value. It keeps the comment bodies of the
// test issue (with the names replaced) and replaces each other body and title
// with a neutral text. Read the output before you commit it.
//
// -only catalog writes only the names of the workspace (catalog*.json) and
// needs no -issue.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/source/linear"
)

// Fixture is one captured response.
type Fixture struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

func main() {
	keyFile := flag.String("key-file", "", "a file with a Linear personal API key")
	issue := flag.String("issue", "", "the identifier of the test issue, for example ENG-123")
	since := flag.Duration("since", 7*24*time.Hour, "read the items of this time before now")
	out := flag.String("out", "internal/source/linear/testdata", "the output folder")
	endpoint := flag.String("endpoint", config.LinearEndpoint, "the GraphQL endpoint")
	only := flag.String("only", "", "write only this file, for example catalog.json")
	flag.Parse()
	if *keyFile == "" || *issue == "" && !strings.HasPrefix(*only, "catalog") {
		log.Fatal("give -key-file and -issue")
	}
	raw, err := os.ReadFile(*keyFile)
	if err != nil {
		log.Fatal(err)
	}
	key := strings.TrimSpace(string(raw))
	from := time.Now().Add(-*since)
	var inboxAfter string
	var catalogNext map[string]any

	cases := map[string]func() (int, []byte){
		"viewer.json":     func() (int, []byte) { return post(*endpoint, key, linear.ViewerQuery, nil) },
		"auth_error.json": func() (int, []byte) { return post(*endpoint, "lin_api_invalid", linear.ViewerQuery, nil) },
		// The default feeds of a new account in one request, then the next
		// page of the inbox.
		"poll.json": func() (int, []byte) {
			doc, vars := linear.PollQuery([]linear.Part{
				{Kind: config.FeedComments, Since: from, Prefix: "/agent", MeOnly: true},
				{Kind: config.FeedInbox, Since: from},
			}, 50)
			st, b := post(*endpoint, key, doc, vars)
			inboxAfter = endCursor(b, 1)
			return st, b
		},
		"poll2.json": func() (int, []byte) {
			doc, vars := linear.PollQuery([]linear.Part{{Kind: config.FeedInbox, Since: from, After: inboxAfter}}, 50)
			return post(*endpoint, key, doc, vars)
		},
		// The comments of the user with no prefix rule: replies and other bodies.
		// The names of the workspace for the feed form, first page of each list.
		"catalog.json": func() (int, []byte) {
			return post(*endpoint, key, linear.CatalogQuery, map[string]any{
				"first": 250, "teams": true, "projects": true, "labels": true, "users": true})
		},
		// Two pages of the names, two on each page: the second request reads
		// only the lists with more pages.
		"catalog_page1.json": func() (int, []byte) {
			st, b := post(*endpoint, key, linear.CatalogQuery, map[string]any{
				"first": 2, "teams": true, "projects": true, "labels": true, "users": true})
			catalogNext = nextNames(b)
			return st, b
		},
		"catalog_page2.json": func() (int, []byte) {
			return post(*endpoint, key, linear.CatalogQuery, catalogNext)
		},
		"mine.json": func() (int, []byte) {
			doc, vars := linear.PollQuery([]linear.Part{{Kind: config.FeedComments, Since: from, MeOnly: true}}, 50)
			return post(*endpoint, key, doc, vars)
		},
	}
	// Two pages of the comments of the user, three on each page.
	var after string
	cases["page1.json"] = func() (int, []byte) {
		doc, vars := linear.PollQuery([]linear.Part{{Kind: config.FeedComments, Since: from, MeOnly: true}}, 3)
		st, b := post(*endpoint, key, doc, vars)
		after = endCursor(b, 0)
		return st, b
	}
	cases["page2.json"] = func() (int, []byte) {
		doc, vars := linear.PollQuery([]linear.Part{{Kind: config.FeedComments, Since: from, MeOnly: true, After: after}}, 3)
		return post(*endpoint, key, doc, vars)
	}

	names := make([]string, 0, len(cases))
	for n := range cases {
		names = append(names, n)
	}
	sort.Strings(names) // page1 before page2
	if *only != "" {
		names = slices.DeleteFunc(names, func(n string) bool { return !strings.HasPrefix(n, *only) })
		if len(names) == 0 {
			log.Fatalf("no case %s", *only)
		}
	}
	got := map[string]Fixture{}
	s := newScrubber(*issue)
	for _, n := range names {
		st, b := cases[n]()
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			log.Fatalf("%s: HTTP %d: %v: %s", n, st, err, b)
		}
		s.learn(v, nil)
		got[n] = Fixture{Status: st, Body: b}
	}
	for _, n := range names {
		var v any
		_ = json.Unmarshal(got[n].Body, &v)
		body, err := json.MarshalIndent(s.walk(v, nil), "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		body = []byte(s.text(string(body)))
		f, _ := json.MarshalIndent(Fixture{Status: got[n].Status, Body: body}, "", "  ")
		if err := os.WriteFile(filepath.Join(*out, n), append(f, '\n'), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: HTTP %d, %d bytes\n", n, got[n].Status, len(f))
	}
}

// nextNames returns the variables of the next page of a catalog response:
// each list with more pages gets its end cursor.
func nextNames(b []byte) map[string]any {
	var r struct {
		Data map[string]*linear.Page `json:"data"`
	}
	_ = json.Unmarshal(b, &r)
	vars := map[string]any{"first": 2}
	for l, field := range map[string]string{"teams": "teams", "projects": "projects", "labels": "issueLabels", "users": "users"} {
		p := r.Data[field]
		more := p != nil && p.PageInfo.HasNextPage
		vars[l] = more
		if more {
			vars[l+"After"] = p.PageInfo.EndCursor
		}
	}
	return vars
}

// endCursor returns the end cursor of part i of a poll response.
func endCursor(b []byte, i int) string {
	var r struct {
		Data map[string]linear.Page `json:"data"`
	}
	_ = json.Unmarshal(b, &r)
	return r.Data[linear.Alias(i)].PageInfo.EndCursor
}

func post(endpoint, key, doc string, vars map[string]any) (int, []byte) {
	payload, _ := json.Marshal(map[string]any{"query": doc, "variables": vars})
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Authorization", key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	return resp.StatusCode, b
}

// scrubber replaces the personal data with fake values. The same real value
// always gets the same fake value, so the links between items stay.
type scrubber struct {
	issue   string            // the identifier of the test issue
	fake    map[string]string // real value -> fake value, also for text
	counter map[string]int
}

func newScrubber(issue string) *scrubber {
	return &scrubber{issue: issue, fake: map[string]string{}, counter: map[string]int{}}
}

func (s *scrubber) next(kind string) int { s.counter[kind]++; return s.counter[kind] }

func (s *scrubber) set(real, fake string) string {
	if real == "" {
		return ""
	}
	if f, ok := s.fake[real]; ok {
		return f
	}
	s.fake[real] = fake
	return fake
}

var identRe = regexp.MustCompile(`^([A-Z][A-Z0-9]*)-(\d+)$`)

// learn records the fake value of each name, key and identifier before the
// walk, so a text can use the names of any item.
func (s *scrubber) learn(v any, path []string) {
	switch t := v.(type) {
	case map[string]any:
		if t["isMe"] == true {
			if dn, ok := t["displayName"].(string); ok {
				s.set(dn, "me.user")
			}
			if n, ok := t["name"].(string); ok {
				s.set(n, "Me User")
			}
			if e, ok := t["email"].(string); ok {
				s.set(e, "me.user@example.com")
			}
		}
		for k, c := range t {
			s.learn(c, append(path, k))
		}
		if org, ok := t["organization"].(map[string]any); ok {
			if k, ok := org["urlKey"].(string); ok {
				s.set(k, "acme")
			}
			if n, ok := org["name"].(string); ok {
				s.set(n, "Acme")
			}
		}
	case []any:
		for _, c := range t {
			s.learn(c, path)
		}
	case string:
		if len(path) == 0 {
			return
		}
		switch k, parent := path[len(path)-1], parentKey(path); {
		case k == "displayName":
			n := s.next("user")
			s.set(t, fmt.Sprintf("user%d", n))
		case k == "identifier":
			if m := identRe.FindStringSubmatch(t); m != nil {
				s.set(m[1], "ENG")
				s.set(t, fmt.Sprintf("ENG-%d", 100+s.next("issue")))
			}
		case k == "key" && parent == "team":
			s.set(t, "ENG")
		case k == "key" && hasKey(path, "teams"):
			s.set(t, fmt.Sprintf("T%d", s.next("teamkey")))
		case k == "name" && hasKey(path, "teams"):
			s.set(t, fmt.Sprintf("Team %d", s.next("team")))
		case k == "name" && (parent == "project" || hasKey(path, "projects")):
			s.set(t, fmt.Sprintf("Project %d", s.next("project")))
		case k == "name" && parent == "nodes" && (hasKey(path, "labels") || hasKey(path, "issueLabels")):
			s.set(t, fmt.Sprintf("Label %d", s.next("label")))
		}
	}
}

func parentKey(path []string) string {
	if len(path) < 2 {
		return ""
	}
	return path[len(path)-2]
}

func hasKey(path []string, k string) bool {
	for _, p := range path {
		if p == k {
			return true
		}
	}
	return false
}

var uuidRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// walk returns a copy of v with the personal data replaced.
func (s *scrubber) walk(v any, path []string) any {
	switch t := v.(type) {
	case map[string]any:
		test := false
		if is, ok := t["issue"].(map[string]any); ok && is["identifier"] == s.issue {
			test = true
		}
		out := map[string]any{}
		for k, c := range t {
			if str, ok := c.(string); ok && (k == "body" || k == "title") {
				out[k] = s.textField(k, str, test || t["identifier"] == s.issue)
				continue
			}
			if k == "parent" && test {
				// The parent of a test comment is in the test issue too.
				if pm, ok := c.(map[string]any); ok {
					pm = s.walk(pm, append(path, k)).(map[string]any)
					if b, ok := c.(map[string]any)["body"].(string); ok {
						pm["body"] = s.textField("body", b, true)
					}
					out[k] = pm
					continue
				}
			}
			out[k] = s.walk(c, append(path, k))
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, c := range t {
			out[i] = s.walk(c, path)
		}
		return out
	case string:
		if len(path) == 0 {
			return t
		}
		k := path[len(path)-1]
		switch {
		case k == "id" || k == "endCursor":
			return s.set(t, fmt.Sprintf("00000000-0000-4000-8000-%012d", s.next("id")))
		case k == "email":
			return s.set(t, fmt.Sprintf("user%d@example.com", s.next("email")))
		case k == "name" && parentKey(path) == "user" || k == "name" && parentKey(path) == "actor":
			return s.set(t, fmt.Sprintf("User %d", s.next("name")))
		case k == "url":
			return s.url(t)
		case k == "message":
			return t
		}
		if f, ok := s.fake[t]; ok {
			return f
		}
		return t
	}
	return v
}

// textField keeps the text of the test issue and replaces any other text.
func (s *scrubber) textField(k, v string, keep bool) string {
	if keep {
		return s.text(v)
	}
	if k == "title" {
		return s.set(v, fmt.Sprintf("Issue title %d", s.next("title")))
	}
	return fmt.Sprintf("Text of comment %d.", s.next("body"))
}

// text replaces each known real value in a text, the longest first.
func (s *scrubber) text(v string) string {
	reals := make([]string, 0, len(s.fake))
	for r := range s.fake {
		if len(r) >= 3 {
			reals = append(reals, r)
		}
	}
	sort.Slice(reals, func(i, j int) bool { return len(reals[i]) > len(reals[j]) })
	for _, r := range reals {
		v = strings.ReplaceAll(v, r, s.fake[r])
	}
	return v
}

var urlRe = regexp.MustCompile(`^https://linear\.app/([^/]+)/issue/([^/]+)/[^#]*(#comment-([0-9a-f]+))?$`)

// url builds a fake URL of an issue or a comment.
func (s *scrubber) url(v string) string {
	m := urlRe.FindStringSubmatch(v)
	if m == nil {
		return uuidRe.ReplaceAllString(s.text(v), "00000000-0000-4000-8000-000000000000")
	}
	u := "https://linear.app/acme/issue/" + s.text(m[2]) + "/issue"
	if m[4] != "" {
		u += "#comment-" + s.set(m[4], fmt.Sprintf("%08x", s.next("hash")))
	}
	return u
}
