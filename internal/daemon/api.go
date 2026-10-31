package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// The daemon serves a small HTTP API on a Unix socket. The API carries no
// message text and no card text: only counts, states, IDs and errors.

// SocketName is the name of the socket file in the data directory.
const SocketName = "i2a.sock"

// maxSocketPath is the longest path of a Unix socket on macOS (104 bytes with
// the final zero byte).
const maxSocketPath = 103

// runWait is the longest time that a run request waits for the loop.
var runWait = 10 * time.Minute

// SocketPath returns the path of the socket of the data directory, and the
// path of the file that holds it when the path is too long for a socket.
func SocketPath(dataDir string) (socket, pointer string) {
	socket = filepath.Join(dataDir, SocketName)
	if len(socket) <= maxSocketPath {
		return socket, ""
	}
	sum := sha256.Sum256([]byte(dataDir))
	return filepath.Join(os.TempDir(), "i2a-"+hex.EncodeToString(sum[:])[:12]+".sock"), socket + ".path"
}

// FindSocket returns the socket path that a client dials: the path in
// i2a.sock.path when that file exists, else <data>/i2a.sock.
func FindSocket(dataDir string) string {
	if b, err := os.ReadFile(filepath.Join(dataDir, SocketName+".path")); err == nil && len(b) > 0 {
		return string(b)
	}
	return filepath.Join(dataDir, SocketName)
}

// Event is one server-sent event of /v1/events.
type Event struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"` // read, send, ask, reload, run, error
	Account string    `json:"account,omitempty"`
	Space   string    `json:"space,omitempty"`
	Agent   string    `json:"agent,omitempty"`
	Count   int       `json:"count,omitempty"`
	Status  string    `json:"status,omitempty"`
	Detail  string    `json:"detail,omitempty"` // an error or the result of a driver; never message text
}

// Event kinds.
const (
	EventRead   = "read"
	EventSend   = "send"
	EventAsk    = "ask"
	EventReload = "reload"
	EventRun    = "run"
	EventError  = "error"
)

// hub sends each event to all subscribers. A slow subscriber loses events;
// the loop never waits.
type hub struct {
	mu   sync.Mutex
	subs map[chan Event]bool
}

func (h *hub) subscribe() chan Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = map[chan Event]bool{}
	}
	ch := make(chan Event, 64)
	h.subs[ch] = true
	return ch
}

func (h *hub) unsubscribe(ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs, ch)
}

func (h *hub) publish(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// AccountRead is the last read of one account.
type AccountRead struct {
	Account string    `json:"account"`
	At      time.Time `json:"at"`
	New     int       `json:"new"`
	Error   string    `json:"error,omitempty"`
}

// SpaceNext is the next send of one space. At is nil when the space does not
// send (it has no agent, or its schedule has no next time).
type SpaceNext struct {
	Space string     `json:"space"`
	Name  string     `json:"name"`
	Agent string     `json:"agent"`
	At    *time.Time `json:"at,omitempty"`
}

// Status is the answer of GET /v1/status.
type Status struct {
	PID        int           `json:"pid"`
	Version    string        `json:"version"`
	StartedAt  time.Time     `json:"started_at"`
	ConfigFile string        `json:"config_file"`
	Spaces     []SpaceNext   `json:"spaces"`
	Reads      []AccountRead `json:"reads"`
}

// RunRequest is the body of POST /v1/run. No spaces means all spaces.
type RunRequest struct {
	Spaces []string `json:"spaces"`
}

// Batch is one batch of a run.
type Batch struct {
	Space  string `json:"space"`
	Agent  string `json:"agent"`
	Driver string `json:"driver"`
	Count  int    `json:"count"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}

// RunResult is the answer of POST /v1/run.
type RunResult struct {
	Summary string        `json:"summary"` // the text of i2a run --once
	Reads   []AccountRead `json:"reads"`
	Batches []Batch       `json:"batches"`
	Failed  bool          `json:"failed"` // a read or a batch failed
}

// runRequest is a run that the loop does.
type runRequest struct {
	spaces []string
	reply  chan runReply
}

type runReply struct {
	res RunResult
	err error
}

// Server is the API of a running daemon.
type Server struct {
	version   string
	started   time.Time
	cfgFile   string
	runs      chan runRequest
	events    hub
	mu        sync.Mutex
	status    Status // the spaces and the reads; the loop updates them
	listener  net.Listener
	removeOld []string // the files to remove when the server stops
	stopping  chan struct{}
}

// ErrUnknownSpace is the error of a run of a space that does not exist.
var ErrUnknownSpace = errors.New("the space does not exist")

// NewServer returns a server. The daemon loop serves its run requests.
func NewServer(version, configFile string, now time.Time) *Server {
	return &Server{version: version, started: now, cfgFile: configFile, runs: make(chan runRequest), stopping: make(chan struct{})}
}

// Listen opens the socket of the data directory with the mode 0600. The
// caller holds the daemon lock, so an old socket file is from a stopped daemon.
func (s *Server) Listen(dataDir string) (string, error) {
	socket, pointer := SocketPath(dataDir)
	_ = os.Remove(socket)
	_ = os.Remove(filepath.Join(dataDir, SocketName+".path"))
	l, err := net.Listen("unix", socket)
	if err != nil {
		return "", fmt.Errorf("listen on %s: %w", socket, err)
	}
	if err := os.Chmod(socket, 0o600); err != nil {
		l.Close()
		return "", err
	}
	s.removeOld = []string{socket}
	if pointer != "" {
		if err := os.WriteFile(pointer, []byte(socket), 0o600); err != nil {
			l.Close()
			return "", err
		}
		s.removeOld = append(s.removeOld, pointer)
	}
	s.listener = l
	return socket, nil
}

// Serve answers the requests until ctx ends, then removes the socket files.
func (s *Server) Serve(ctx context.Context) error {
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() {
		<-ctx.Done()
		close(s.stopping)
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	err := srv.Serve(s.listener)
	for _, f := range s.removeOld {
		_ = os.Remove(f)
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Handler returns the HTTP handler of the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", s.handleStatus)
	mux.HandleFunc("POST /v1/run", s.handleRun)
	mux.HandleFunc("GET /v1/events", s.handleEvents)
	return mux
}

// Paths lists the paths of the API, for the test of the OpenAPI file.
var Paths = []string{"/v1/status", "/v1/run", "/v1/events"}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	st := s.status
	s.mu.Unlock()
	st.PID, st.Version, st.StartedAt, st.ConfigFile = os.Getpid(), s.version, s.started, s.cfgFile
	if st.Spaces == nil {
		st.Spaces = []SpaceNext{}
	}
	if st.Reads == nil {
		st.Reads = []AccountRead{}
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req RunRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "the body is not {\"spaces\": [...]}: "+err.Error())
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), runWait)
	defer cancel()
	rr := runRequest{spaces: req.Spaces, reply: make(chan runReply, 1)}
	select {
	case s.runs <- rr:
	case <-s.stopping:
		writeError(w, http.StatusServiceUnavailable, "the daemon is stopping: no run")
		return
	case <-ctx.Done():
		writeError(w, http.StatusServiceUnavailable, "the daemon is busy: try again")
		return
	}
	select {
	case rep := <-rr.reply:
		switch {
		case errors.Is(rep.err, ErrUnknownSpace):
			writeError(w, http.StatusBadRequest, rep.err.Error())
		case rep.err != nil:
			writeError(w, http.StatusInternalServerError, rep.err.Error())
		default:
			writeJSON(w, http.StatusOK, rep.res)
		}
	case <-s.stopping:
		writeError(w, http.StatusServiceUnavailable, "the daemon is stopping: the run stops with it")
	case <-ctx.Done():
		writeError(w, http.StatusGatewayTimeout, "the run did not end in time; it continues in the daemon")
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "no streaming")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": i2a events\n\n")
	fl.Flush()
	ch := s.events.subscribe()
	defer s.events.unsubscribe(ch)
	keep := time.NewTicker(30 * time.Second)
	defer keep.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keep.C:
			fmt.Fprint(w, ": keep\n\n")
		case ev := <-ch:
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, b)
		}
		fl.Flush()
	}
}

// Publish sends an event to the subscribers of /v1/events.
func (s *Server) Publish(ev Event) {
	if s == nil {
		return
	}
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	s.events.publish(ev)
}

// setStatus replaces the spaces or the reads of the status. nil keeps the old value.
func (s *Server) setStatus(spaces []SpaceNext, reads []AccountRead) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if spaces != nil {
		s.status.Spaces = spaces
	}
	if reads != nil {
		s.status.Reads = reads
	}
}
