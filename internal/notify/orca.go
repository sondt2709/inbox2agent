package notify

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// The i2a plugin of Orca (extensions/orca) and its command.
const (
	OrcaPluginKey = "inbox2agent.i2a"
	OrcaCommand   = "notify"
	orcaTimeout   = 3 * time.Second
)

// Orca shows a note through the i2a plugin of Orca. It calls the plugin
// command with the runtime RPC plugins.invokeCommand on the Unix socket that
// orca-runtime.json names. The plugin system of Orca is experimental and off
// by default; each error is a failure, and the chain goes on.
type Orca struct {
	UserData string // the Orca user data folder; "" is OrcaUserData()
}

// Name implements Sender.
func (Orca) Name() string { return ViaOrca }

// OrcaUserData returns $ORCA_USER_DATA_PATH, or the folder of the Orca app.
func OrcaUserData() string {
	if v := os.Getenv("ORCA_USER_DATA_PATH"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "orca")
}

type orcaRuntime struct {
	AuthToken  string `json:"authToken"`
	Transports []struct {
		Kind     string `json:"kind"`
		Endpoint string `json:"endpoint"`
	} `json:"transports"`
}

// orcaSocket reads orca-runtime.json and returns the Unix socket and the token.
func orcaSocket(dir string) (string, string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "orca-runtime.json"))
	if err != nil {
		return "", "", fmt.Errorf("orca is not running: %w", err)
	}
	var rt orcaRuntime
	if err := json.Unmarshal(b, &rt); err != nil {
		return "", "", errors.New("orca-runtime.json is not valid JSON")
	}
	for _, t := range rt.Transports {
		if t.Kind == "unix" && t.Endpoint != "" {
			return t.Endpoint, rt.AuthToken, nil
		}
	}
	return "", "", errors.New("orca-runtime.json has no unix transport")
}

// OrcaArgs returns the arguments of the plugin command for one note. Orca
// has no subtitle, so the subtitle goes before the body.
func OrcaArgs(n Note) map[string]string {
	body := Clean(n.Body)
	if s := Clean(n.Subtitle); s != "" {
		body = s + " — " + body
	}
	return map[string]string{"title": Clean(n.Title), "body": body}
}

// Send implements Sender.
func (o Orca) Send(ctx context.Context, n Note) error {
	dir := o.UserData
	if dir == "" {
		dir = OrcaUserData()
	}
	sock, token, err := orcaSocket(dir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, orcaTimeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", sock)
	if err != nil {
		return fmt.Errorf("orca is not reachable: %w", err)
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	idb := make([]byte, 8)
	_, _ = rand.Read(idb)
	id := "i2a-" + hex.EncodeToString(idb)
	req, err := json.Marshal(map[string]any{
		"id": id, "authToken": token, "method": "plugins.invokeCommand",
		"params": map[string]any{"pluginKey": OrcaPluginKey, "commandId": OrcaCommand, "args": OrcaArgs(n)},
	})
	if err != nil {
		return err
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return fmt.Errorf("orca: write: %w", err)
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		var resp struct {
			ID        string `json:"id"`
			OK        bool   `json:"ok"`
			Keepalive bool   `json:"_keepalive"`
			Error     *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
			return errors.New("orca: the response is not valid JSON")
		}
		if resp.Keepalive || resp.ID != id {
			continue
		}
		if resp.OK {
			return nil
		}
		if resp.Error != nil {
			return fmt.Errorf("orca: %s: %s", resp.Error.Code, resp.Error.Message)
		}
		return errors.New("orca: the request failed")
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("orca: read: %w", err)
	}
	return errors.New("orca closed the connection with no response")
}
