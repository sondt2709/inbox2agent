package linear

import (
	"context"
	"sort"
	"strings"
)

// catalogPage is the number of items on one page of a name list.
const catalogPage = 250

// CatalogQuery reads the names of the workspace for the feed form. Each list
// has its own cursor and an @include flag, so the next request reads only the
// lists that have more pages.
const CatalogQuery = `query Catalog($first: Int!, $teams: Boolean!, $projects: Boolean!, $labels: Boolean!, $users: Boolean!, ` +
	`$teamsAfter: String, $projectsAfter: String, $labelsAfter: String, $usersAfter: String) {
  teams(first: $first, after: $teamsAfter) @include(if: $teams) { nodes { key name } pageInfo { hasNextPage endCursor } }
  projects(first: $first, after: $projectsAfter) @include(if: $projects) { nodes { name } pageInfo { hasNextPage endCursor } }
  issueLabels(first: $first, after: $labelsAfter) @include(if: $labels) { nodes { name isGroup } pageInfo { hasNextPage endCursor } }
  users(first: $first, after: $usersAfter) @include(if: $users) { nodes { displayName active } pageInfo { hasNextPage endCursor } }
}`

// Team is a team of the workspace. A feed filter uses its key.
type Team struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Catalog holds the names of a workspace, each list sorted without case.
type Catalog struct {
	Teams    []Team
	Projects []string
	Labels   []string // without the label groups, each name one time
	Users    []string // the display names of the active users
	// More is true when a list has more than maxPages pages; the lists hold
	// the first pages only.
	More bool
}

type namePage[T any] struct {
	Nodes    []T      `json:"nodes"`
	PageInfo PageInfo `json:"pageInfo"`
}

type catalogResult struct {
	Teams    *namePage[Team] `json:"teams"`
	Projects *namePage[struct {
		Name string `json:"name"`
	}] `json:"projects"`
	Labels *namePage[struct {
		Name    string `json:"name"`
		IsGroup bool   `json:"isGroup"`
	}] `json:"issueLabels"`
	Users *namePage[struct {
		DisplayName string `json:"displayName"`
		Active      bool   `json:"active"`
	}] `json:"users"`
}

// Catalog reads the teams, projects, labels and users of the workspace.
func (src *Source) Catalog(ctx context.Context) (Catalog, error) {
	var c Catalog
	lists := []string{"teams", "projects", "labels", "users"}
	after := map[string]string{}
	open := map[string]bool{"teams": true, "projects": true, "labels": true, "users": true}
	for page := 0; len(open) > 0; page++ {
		if page == src.s.MaxPages {
			c.More = true
			break
		}
		vars := map[string]any{"first": src.s.CatalogPage}
		for _, l := range lists {
			vars[l] = open[l]
			if a := after[l]; a != "" {
				vars[l+"After"] = a
			}
		}
		var r catalogResult
		if err := src.client.Query(ctx, CatalogQuery, vars, &r); err != nil {
			return Catalog{}, err
		}
		next := func(l string, p PageInfo) {
			if p.HasNextPage && p.EndCursor != "" {
				after[l] = p.EndCursor
				return
			}
			delete(open, l)
		}
		if r.Teams != nil {
			c.Teams = append(c.Teams, r.Teams.Nodes...)
			next("teams", r.Teams.PageInfo)
		}
		if r.Projects != nil {
			for _, n := range r.Projects.Nodes {
				c.Projects = append(c.Projects, n.Name)
			}
			next("projects", r.Projects.PageInfo)
		}
		if r.Labels != nil {
			for _, n := range r.Labels.Nodes {
				if !n.IsGroup {
					c.Labels = append(c.Labels, n.Name)
				}
			}
			next("labels", r.Labels.PageInfo)
		}
		if r.Users != nil {
			for _, n := range r.Users.Nodes {
				if n.Active {
					c.Users = append(c.Users, n.DisplayName)
				}
			}
			next("users", r.Users.PageInfo)
		}
		for _, l := range lists {
			if open[l] && after[l] == "" {
				delete(open, l) // the response has no page of this list
			}
		}
	}
	sort.Slice(c.Teams, func(i, j int) bool { return strings.ToLower(c.Teams[i].Key) < strings.ToLower(c.Teams[j].Key) })
	c.Projects, c.Labels, c.Users = uniqueFold(c.Projects), uniqueFold(c.Labels), uniqueFold(c.Users)
	return c, nil
}

// uniqueFold sorts the names without case and keeps each name one time.
func uniqueFold(names []string) []string {
	sort.SliceStable(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	out := names[:0]
	for _, n := range names {
		if n == "" || len(out) > 0 && strings.EqualFold(out[len(out)-1], n) {
			continue
		}
		out = append(out, n)
	}
	return out
}
