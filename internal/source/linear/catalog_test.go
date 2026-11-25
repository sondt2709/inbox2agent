package linear

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestCatalog(t *testing.T) {
	f := newFake(t, "catalog.json")
	c, err := newSource(f, Settings{}).Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Teams, []Team{{Key: "T1", Name: "Team 1"}, {Key: "T2", Name: "Team 2"}}) {
		t.Errorf("teams = %v", c.Teams)
	}
	// The workspace has 174 labels: one is a label group, and each name is
	// one time without case.
	if len(c.Projects) != 70 || len(c.Labels) != 173 || len(c.Users) != 42 || c.More {
		t.Errorf("projects %d, labels %d, users %d, more %v", len(c.Projects), len(c.Labels), len(c.Users), c.More)
	}
	if !slices.Equal(c.Projects[:3], []string{"Project 1", "Project 10", "Project 11"}) || c.Labels[0] != "Label 1" {
		t.Errorf("the names are not sorted without case: %v, %v", c.Projects[:3], c.Labels[:3])
	}
	for i := 1; i < len(c.Labels); i++ {
		if strings.EqualFold(c.Labels[i-1], c.Labels[i]) {
			t.Errorf("the label %q is two times", c.Labels[i])
		}
	}
	if len(f.reqs) != 1 || f.reqs[0].Variables["first"] != float64(catalogPage) || f.keys[0] != "lin_api_test" {
		t.Fatalf("requests = %+v", f.reqs)
	}
	if !isQuery(f.reqs[0].Query) || !strings.HasPrefix(f.reqs[0].Query, "query Catalog") {
		t.Errorf("the request is not only a query:\n%s", f.reqs[0].Query)
	}
}

func TestCatalogPages(t *testing.T) {
	// Page 1: the teams are complete; projects, labels and users have more.
	f := newFake(t, "catalog_page1.json", "catalog_page2.json")
	c, err := newSource(f, Settings{CatalogPage: 2, MaxPages: 2}).Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(f.reqs))
	}
	v := f.reqs[1].Variables
	want := map[string]any{"first": float64(2), "teams": false, "projects": true, "labels": true, "users": true,
		"projectsAfter": "00000000-0000-4000-8000-000000000006", "labelsAfter": "00000000-0000-4000-8000-000000000007",
		"usersAfter": "00000000-0000-4000-8000-000000000008"}
	for k, w := range want {
		if v[k] != w {
			t.Errorf("page 2: %s = %v, want %v", k, v[k], w)
		}
	}
	if _, ok := v["teamsAfter"]; ok {
		t.Error("page 2 reads the teams again")
	}
	if len(c.Teams) != 2 || !slices.Equal(c.Projects, []string{"Project 1", "Project 2", "Project 3", "Project 4"}) ||
		len(c.Labels) != 4 || len(c.Users) != 4 {
		t.Errorf("catalog = %+v", c)
	}
	// Page 2 still has more pages, and MaxPages is 2.
	if !c.More {
		t.Error("More is false after the page limit")
	}
}

func TestUniqueFold(t *testing.T) {
	// Two teams can have a label with one name.
	got := uniqueFold([]string{"infra", "Bug", "Infra", "", "bug", "API"})
	if !slices.Equal(got, []string{"API", "Bug", "infra"}) {
		t.Errorf("uniqueFold = %v", got)
	}
}
