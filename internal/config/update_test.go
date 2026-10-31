package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func addSpace(id string) func(*Config) error {
	return func(c *Config) error {
		c.Spaces = append(c.Spaces, Space{ID: id, Name: id, Agent: "triage", Schedule: "always"})
		return nil
	}
}

func TestUpdateKeepsConcurrentChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := Update(path, addSpace(fmt.Sprintf("s%d", i)))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Spaces) != 8 {
		t.Fatalf("spaces after 8 concurrent updates: %d, want 8", len(c.Spaces))
	}
}

func TestUpdateReturnsTheHashOfTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	c, hash, err := Update(path, addSpace("acme"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Space("acme"); !ok {
		t.Error("the returned configuration has no space acme")
	}
	b, _ := os.ReadFile(path)
	if hash != Hash(b) {
		t.Errorf("hash %s is not the hash of the file %s", hash, Hash(b))
	}
}

func TestUpdateWritesNothingWhenTheChangeFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	_, _, err := Update(path, func(c *Config) error {
		c.Spaces = append(c.Spaces, Space{ID: "x", Name: "x"})
		return MissingError{Kind: KindAgent, ID: "gone"}
	})
	var me MissingError
	if !errors.As(err, &me) || me.ID != "gone" {
		t.Fatalf("err = %v, want MissingError for gone", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("a failed change wrote the file")
	}
}

func TestUpdateFailsWhenTheLockIsHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	unlock, err := lock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	old := lockWait
	lockWait = 200 * time.Millisecond
	defer func() { lockWait = old }()
	if _, _, err := Update(path, addSpace("acme")); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
	if ErrLocked.Error() != "config.toml is locked by another process" {
		t.Errorf("error text %q", ErrLocked.Error())
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("the update wrote the file without the lock")
	}
	fi, err := os.Stat(LockFile(path))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("lock file mode: %v %v, want 0600", fi.Mode().Perm(), err)
	}
}

func TestUpdateWithoutFileUsesTheDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	c, _, err := Update(path, addSpace("acme"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Agents) != 1 || c.Agents[0].ID != "triage" {
		t.Errorf("agents %v, want the default triage", c.Agents)
	}
}

func TestSaveWritesTheHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), SchemaLine+Header) || !strings.Contains(Header, "removes other comments") {
		t.Errorf("the file does not start with the header:\n%s", b)
	}
}

func TestItem(t *testing.T) {
	c := Default()
	c.Accounts = []Account{NewMattermostAccount("work", "https://chat.example.com")}
	c.Spaces = []Space{{ID: "acme", Name: "Acme", Agent: "triage", Schedule: "always",
		Conversations: []Conversation{{Account: "work", ID: "C1", Read: ReadAll}}}}
	for _, tc := range []struct{ kind, id string }{
		{KindSettings, ""}, {KindUnsorted, ""}, {KindAccount, "work"}, {KindAgent, "triage"},
		{KindSchedule, "always"}, {KindSpace, "acme"}, {KindConversation, "work/C1"},
	} {
		if _, ok := Item(c, tc.kind, tc.id); !ok {
			t.Errorf("Item(%s, %q) not found", tc.kind, tc.id)
		}
	}
	for _, tc := range []struct{ kind, id string }{
		{KindAccount, "x"}, {KindAgent, "x"}, {KindSchedule, "x"}, {KindSpace, "x"}, {KindConversation, "work/x"}, {"other", ""},
	} {
		if _, ok := Item(c, tc.kind, tc.id); ok {
			t.Errorf("Item(%s, %q) found an item that does not exist", tc.kind, tc.id)
		}
	}
	// A space without its conversations: a moved conversation is not a change of the space.
	before, _ := Item(c, KindSpace, "acme")
	c.Spaces[0].Conversations = nil
	if after, _ := Item(c, KindSpace, "acme"); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Error("a change of the conversations changed the space item")
	}
	c.Spaces[0].Agent = "other"
	if after, _ := Item(c, KindSpace, "acme"); fmt.Sprint(after) == fmt.Sprint(before) {
		t.Error("a change of the agent did not change the space item")
	}
}
