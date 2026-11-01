package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

// SeenFile is the file in the data folder with the conversations that the
// dashboard showed, by account. A conversation that an account lists, that the
// configuration does not have, and that is not in this file is new. The file
// is the state of the dashboard, not of the configuration.
const SeenFile = "conversations-seen.json"

// seenSet holds the shown conversation IDs of each account.
type seenSet map[string][]string

func (s seenSet) has(account, id string) bool { return slices.Contains(s[account], id) }

// readSeen reads the file. A missing or bad file is an empty set.
func readSeen(dataDir string) seenSet {
	out := seenSet{}
	b, err := os.ReadFile(filepath.Join(dataDir, SeenFile))
	if err != nil {
		return out
	}
	if json.Unmarshal(b, &out) != nil {
		return seenSet{}
	}
	return out
}

// addSeen joins the IDs with the file of the data folder and writes it, so a
// second dashboard keeps its own IDs. It returns the joined set. seed is true
// for the first listing of an account: the account gets an entry also when it
// lists nothing.
func addSeen(dataDir, account string, ids []string, seed bool) (seenSet, error) {
	cur := readSeen(dataDir)
	_, had := cur[account]
	for _, id := range ids {
		if !slices.Contains(cur[account], id) {
			cur[account] = append(cur[account], id)
		}
	}
	if seed && !had && cur[account] == nil {
		cur[account] = []string{}
	}
	b, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return cur, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return cur, err
	}
	tmp := filepath.Join(dataDir, SeenFile+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return cur, err
	}
	return cur, os.Rename(tmp, filepath.Join(dataDir, SeenFile))
}
