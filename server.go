package astiencoder

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astikit"
	"github.com/asticode/go-astiws"
	"github.com/gorilla/websocket"
	"github.com/julienschmidt/httprouter"
)

type Server struct {
	l  astikit.SeverityLogger
	p  *serverPlayback
	r  *serverRecording
	w  *Workflow
	ws *astiws.Manager
}

func NewServer(l astikit.StdLogger) (s *Server) {
	s = &Server{
		l:  astikit.AdaptStdLogger(l),
		p:  newServerPlayback(),
		ws: astiws.NewManager(astiws.ManagerConfiguration{MaxMessageSize: 8192}, l),
	}
	s.r = newServerRecording(s.l)
	return
}

func (s *Server) SetWorkflow(w *Workflow) {
	s.w = w
}

func (s *Server) Handler() http.Handler {
	// Create router
	r := httprouter.New()

	// Add routes
	r.Handler(http.MethodGet, "/", s.serveHomepage())
	r.Handler(http.MethodGet, "/ok", s.serveOK())
	r.Handler(http.MethodPost, "/playback/load", s.servePlaybackLoad())
	r.Handler(http.MethodGet, "/playback/next", s.servePlaybackNext())
	r.Handler(http.MethodGet, "/playback/unload", s.servePlaybackUnload())
	r.Handler(http.MethodGet, "/recording/export", s.serveRecordingExport())
	r.Handler(http.MethodGet, "/recording/start", s.serveRecordingStart())
	r.Handler(http.MethodGet, "/recording/stop", s.serveRecordingStop())
	r.Handler(http.MethodGet, "/websocket", s.serveWebSocket())
	r.Handler(http.MethodGet, "/welcome", s.serveWelcome())
	return r
}

func (s *Server) serveOK() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {})
}

func (s *Server) serveWebSocket() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if err := s.ws.ServeHTTP(rw, r, s.adaptWebSocketClient); err != nil {
			var e *websocket.CloseError
			if ok := errors.As(err, &e); !ok ||
				(e.Code != websocket.CloseNoStatusReceived && e.Code != websocket.CloseNormalClosure) {
				s.l.Error(fmt.Errorf("astiencoder: handling websocket failed: %w", err))
			}
			return
		}
	})
}

func (s *Server) adaptWebSocketClient(c *astiws.Client) (err error) {
	// Register client
	s.ws.AutoRegisterClient(c)

	// Add listeners
	c.AddListener(astiws.EventNameDisconnect, s.webSocketDisconnected)
	c.AddListener("ping", s.webSocketPing)
	return
}

func (s *Server) webSocketDisconnected(c *astiws.Client, eventName string, payload json.RawMessage) error {
	s.ws.UnregisterClient(c)
	return nil
}

func (s *Server) webSocketPing(c *astiws.Client, eventName string, payload json.RawMessage) error {
	if err := c.ExtendConnection(); err != nil {
		s.l.Error(fmt.Errorf("astiencoder: extending ws connection failed: %w", err))
	}
	return nil
}

func (s *Server) sendWebSocket(eventName string, payload interface{}) {
	// Loop through clients
	s.ws.Loop(func(_ interface{}, c *astiws.Client) {
		if err := c.Write(eventName, payload); err != nil {
			s.l.Error(fmt.Errorf("astiencoder: writing event %s with payload %+v to websocket client %p failed: %w", eventName, payload, c, err))
			return
		}
	})
}

func (s *Server) EventHandlerAdapter(eh *EventHandler) {
	// Register catch all handler
	eh.AddForAll(func(e Event) bool {
		// Get payload
		var p interface{}
		switch e.Name {
		case EventNameError:
			p = astikit.ErrorCause(e.Payload.(error))
		case EventNameNodeStats, EventNameWorkflowStats:
			p = newServerStats(e)
		case EventNameNodeContinued, EventNameNodePaused, EventNameNodeStopped:
			p = e.Target.(Node).Metadata().Name
		case EventNameNodeStarted:
			p = newServerNode(e.Target.(Node))
		}

		// Add to recording
		if err := s.r.add(e.Name, p); err != nil {
			s.l.Error(fmt.Errorf("astiencoder: adding to recording failed: %w", err))
		}

		// Send if playback not loaded
		if s.p.r == nil {
			s.sendWebSocket(e.Name, p)
		}
		return false
	})
}

type ServerWorkflow struct {
	Name   string       `json:"name"`
	Nodes  []ServerNode `json:"nodes"`
	Status string       `json:"status"`
}

func (s *Server) newServerWorkflow() (w *ServerWorkflow) {
	// Create server workflow
	w = &ServerWorkflow{
		Name:   s.w.Name(),
		Nodes:  []ServerNode{},
		Status: s.w.Status(),
	}

	// Discover nodes
	ns := make(map[string]ServerNode)
	for _, n := range s.w.Children() {
		discoverServerNode(n, ns)
	}

	// Add nodes
	for _, n := range ns {
		w.Nodes = append(w.Nodes, n)
	}
	return
}

func discoverServerNode(n Node, ns map[string]ServerNode) {
	// Node has already been discovered
	if _, ok := ns[n.Metadata().Name]; ok {
		return
	}

	// Create server node
	ns[n.Metadata().Name] = newServerNode(n)

	// Discover children
	for _, n := range n.Children() {
		discoverServerNode(n, ns)
	}
}

type ServerNode struct {
	Children    []string `json:"children"`
	Description string   `json:"description"`
	Label       string   `json:"label"`
	Name        string   `json:"name"`
	Parents     []string `json:"parents"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags"`
}

func newServerNode(n Node) (s ServerNode) {
	// Create node
	s = ServerNode{
		Children:    []string{},
		Description: n.Metadata().Description,
		Label:       n.Metadata().Label,
		Name:        n.Metadata().Name,
		Parents:     []string{},
		Status:      n.Status(),
		Tags:        n.Metadata().Tags,
	}

	// Add children
	for _, n := range n.Children() {
		s.Children = append(s.Children, n.Metadata().Name)
	}

	// Add parents
	for _, n := range n.Parents() {
		s.Parents = append(s.Parents, n.Metadata().Name)
	}
	return
}

type ServerStats struct {
	Name  string       `json:"name"`
	Stats []ServerStat `json:"stats"`
}

func newServerStats(e Event) (s ServerStats) {
	if e.Name == EventNameNodeStats {
		s.Name = e.Target.(Node).Metadata().Name
	} else {
		s.Name = e.Target.(*Workflow).Name()
	}
	for _, es := range e.Payload.([]EventStat) {
		s.Stats = append(s.Stats, newServerStat(es))
	}
	return
}

type ServerStat struct {
	Description string      `json:"description"`
	Label       string      `json:"label"`
	Unit        string      `json:"unit"`
	Value       interface{} `json:"value"`
}

func newServerStat(e EventStat) ServerStat {
	return ServerStat{
		Description: e.Description,
		Label:       e.Label,
		Unit:        e.Unit,
		Value:       e.Value,
	}
}

type ServerWelcome struct {
	Recording bool            `json:"recording"`
	Workflow  *ServerWorkflow `json:"workflow,omitempty"`
}

func (s *Server) serveWelcome() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Playback is loaded
		if s.p.r != nil {
			// Unload playback
			if err := s.p.unload(); err != nil {
				s.l.Error(fmt.Errorf("astiencoder: unloading playback failed: %w", err))
				rw.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

		// Create body
		b := ServerWelcome{Recording: s.r.w != nil}

		// Add workflow
		if s.w != nil {
			b.Workflow = s.newServerWorkflow()
		}

		// Write
		if err := json.NewEncoder(rw).Encode(b); err != nil {
			s.l.Error(fmt.Errorf("astiencoder: writing failed: %w", err))
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

type serverRecording struct {
	c    *astikit.Chan
	l    astikit.SeverityLogger
	path string
	s    uint32
	w    *csv.Writer
}

func newServerRecording(l astikit.SeverityLogger) *serverRecording {
	return &serverRecording{
		c: astikit.NewChan(astikit.ChanOptions{
			ProcessAll: true,
		}),
		l: l,
	}
}

func (s *Server) serveRecordingStart() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Start
		if err := s.StartRecording("", nil); err != nil {
			s.l.Error(err)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

// StartRecording starts the recording
func (s *Server) StartRecording(dst string, onDone func(path string) error) (err error) {
	// Start recording
	if err = s.r.start(dst, *s.newServerWorkflow(), onDone); err != nil {
		err = fmt.Errorf("astiencoder: starting recording failed: %w", err)
		return
	}
	return
}

func (r *serverRecording) start(dst string, sw ServerWorkflow, onDone func(path string) error) (err error) {
	// Recording already started
	if started := atomic.LoadUint32(&r.s); started > 0 {
		return
	}

	// Create destination
	var f *os.File
	if dst != "" {
		if f, err = os.Create(dst); err != nil {
			err = fmt.Errorf("astiencoder: creating %s failed: %w", dst, err)
			return
		}
	} else {
		if f, err = ioutil.TempFile("", "astiencoder*.csv"); err != nil {
			err = fmt.Errorf("astiencoder: creating temp file failed: %w", err)
			return
		}
	}

	// Update path
	r.path = f.Name()

	// Create csv writer
	r.w = csv.NewWriter(f)

	// Marshal server workflow
	var b []byte
	if b, err = json.Marshal(sw); err != nil {
		err = fmt.Errorf("astiencoder: marshaling failed: %w", err)
		return
	}

	// Write server workflow
	r.w.Write([]string{"", "", string(b)})

	// Execute the rest in a goroutine
	go func() {
		// Start chan
		r.c.Start(context.Background())

		// Reset chan
		r.c.Reset()

		// Flush csv
		r.w.Flush()

		// Reset csv writer
		r.w = nil

		// Close file
		if err := f.Close(); err != nil {
			r.l.Error(fmt.Errorf("astiencoder: closing file failed: %w", err))
			return
		}

		// On done
		if onDone != nil {
			if err := onDone(r.path); err != nil {
				r.l.Error(fmt.Errorf("astiencoder: on done failed: %w", err))
				return
			}
		}
	}()

	// Update started
	atomic.StoreUint32(&r.s, 1)
	return
}

func (r *serverRecording) add(name string, payload interface{}) (err error) {
	// Recording not started
	if started := atomic.LoadUint32(&r.s); started == 0 {
		return
	}

	// Marshal payload
	var b []byte
	if b, err = json.Marshal(payload); err != nil {
		err = fmt.Errorf("astiencoder: marshaling failed: %w", err)
		return
	}

	// Write
	r.c.Add(func() {
		r.w.Write([]string{strconv.Itoa(int(time.Now().UTC().Unix())), name, string(b)})
		r.w.Flush()
	})
	return
}

func (s *Server) serveRecordingStop() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Stop recording
		s.StopRecording()
	})
}

// StopRecording stops the recording
func (s *Server) StopRecording() {
	// Stop recording
	s.r.stop()
}

func (r *serverRecording) stop() {
	// Recording not started
	if started := atomic.LoadUint32(&r.s); started == 0 {
		return
	}

	// Update started
	atomic.StoreUint32(&r.s, 0)

	// Stop chan
	r.c.Stop()
}

func (s *Server) serveRecordingExport() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Export
		if err := s.r.export(rw, s.w); err != nil {
			s.l.Error(fmt.Errorf("astiencoder: exporting recording failed: %w", err))
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

func (r *serverRecording) export(rw http.ResponseWriter, w *Workflow) (err error) {
	// No path
	if r.path == "" {
		return
	}

	// Open file
	var f *os.File
	if f, err = os.Open(r.path); err != nil {
		err = fmt.Errorf("astiencoder: opening %s failed: %w", r.path, err)
		return
	}

	// Stat
	var fi os.FileInfo
	if fi, err = f.Stat(); err != nil {
		f.Close()
		err = fmt.Errorf("astiencoder: stating %s failed: %w", r.path, err)
		return
	}

	// Set headers
	rw.Header().Set("Content-Type", "text/csv")
	rw.Header().Set("Content-Length", strconv.Itoa(int(fi.Size())))
	rw.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(w.Name()+"-"+time.Now().UTC().Format("2006_01_02_15_04_05.csv")))

	// Copy
	if _, err = io.Copy(rw, f); err != nil {
		f.Close()
		err = fmt.Errorf("astiencoder: copying failed: %w", err)
		return
	}

	// Close
	if err = f.Close(); err != nil {
		err = fmt.Errorf("astiencoder: closing failed: %w", err)
		return
	}

	// Remove
	if err = os.Remove(r.path); err != nil {
		err = fmt.Errorf("astiencoder: removing %s failed: %w", r.path, err)
		return
	}
	return
}

type serverPlayback struct {
	b []serverPlaybackItem
	f *os.File
	r *csv.Reader
}

type serverPlaybackItem struct {
	name    string
	payload []byte
	time    time.Time
}

func newServerPlayback() *serverPlayback {
	return &serverPlayback{}
}

func (p *serverPlayback) load(req *http.Request) (payload []byte, err error) {
	// Playbac already loaded
	if p.r != nil {
		return
	}

	// Get request file
	var f multipart.File
	var h *multipart.FileHeader
	if f, h, err = req.FormFile("file"); err != nil {
		if err != http.ErrMissingFile {
			err = fmt.Errorf("astiencoder: getting request file failed: %w", err)
		} else {
			err = fmt.Errorf("astiencoder: request file is missing")
		}
		return
	}

	// Invalid content type
	if c := h.Header.Get("Content-Type"); c != "text/csv" {
		err = fmt.Errorf("astiencoder: invalid content type %s", c)
		return
	}

	// Create file
	if p.f, err = ioutil.TempFile("", "astiencoder*.csv"); err != nil {
		err = fmt.Errorf("astiencoder: creating file failed: %w", err)
		return
	}

	// Copy
	if _, err = io.Copy(p.f, f); err != nil {
		err = fmt.Errorf("astiencoder: copying failed: %w", err)
		return
	}

	// Rewind
	if _, err = p.f.Seek(0, 0); err != nil {
		err = fmt.Errorf("astiencoder: seeking failed: %w", err)
		return
	}

	// Create csv reader
	p.r = csv.NewReader(p.f)

	// Read
	var line []string
	if line, err = p.r.Read(); err != nil {
		err = fmt.Errorf("astiencoder: reading failed: %w", err)
		return
	}

	// Not enough columns
	if len(line) < 3 {
		err = fmt.Errorf("astiencoder: line have only %d columns", len(line))
		return
	}

	// Parse payload
	payload = []byte(line[2])
	return
}

func (p *serverPlayback) next() (i serverPlaybackItem, err error) {
	// Playback not loaded
	if p.r == nil {
		return
	}

	// Check buffer first
	if len(p.b) > 0 {
		i = p.b[0]
		p.b = p.b[1:]
		return
	}

	// Read
	var line []string
	if line, err = p.r.Read(); err != nil {
		err = fmt.Errorf("astiencoder: reading failed: %w", err)
		return
	}

	// Not enough columns
	if len(line) < 3 {
		err = fmt.Errorf("astiencoder: line have only %d columns", len(line))
		return
	}

	// Parse time
	var ti int
	if ti, err = strconv.Atoi(line[0]); err != nil {
		err = fmt.Errorf("astiencoder: atoi of %s failed: %w", line[0], err)
		return
	}
	i.time = time.Unix(int64(ti), 0)

	// Parse name and payload
	i.name = line[1]
	i.payload = []byte(line[2])
	return
}

func (p *serverPlayback) unload() (err error) {
	// Playback not loaded
	if p.r == nil {
		return
	}

	// Close file
	if err = p.f.Close(); err != nil {
		err = fmt.Errorf("astiencoder: closing previous file failed: %w", err)
		return
	}

	// Remove file
	if err = os.Remove(p.f.Name()); err != nil {
		err = fmt.Errorf("astiencoder: removing previous file failed: %w", err)
		return
	}

	// Reset
	p.f = nil
	p.r = nil
	return
}

func (s *Server) servePlaybackLoad() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Load
		payload, err := s.p.load(r)
		if err != nil {
			s.l.Error(fmt.Errorf("astiencoder: loading playback failed: %w", err))
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Write
		if _, err = rw.Write(payload); err != nil {
			s.l.Error(fmt.Errorf("astiencoder: writing failed: %w", err))
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

type ServerPlaybackItems struct {
	Done  bool                 `json:"done"`
	Items []ServerPlaybackItem `json:"items"`
}

type ServerPlaybackItem struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

func newServerPlaybackItem(i serverPlaybackItem) ServerPlaybackItem {
	return ServerPlaybackItem{
		Name:    i.name,
		Payload: i.payload,
	}
}

func (s *Server) servePlaybackNext() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Create body
		b := ServerPlaybackItems{
			Items: []ServerPlaybackItem{},
		}

		// Loop
		var firstAt time.Time
		for {
			// Next
			i, err := s.p.next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					b.Done = true
					break
				} else {
					s.l.Error(fmt.Errorf("astiencoder: next failed: %w", err))
					rw.WriteHeader(http.StatusInternalServerError)
					return
				}
			}

			// Create first at
			if firstAt.IsZero() {
				firstAt = i.time
			}

			// Event is too old
			if i.time.Sub(firstAt) > 500*time.Millisecond {
				// Append in buffer
				s.p.b = append(s.p.b, i)
				break
			}

			// Append event
			b.Items = append(b.Items, newServerPlaybackItem(i))
		}

		// Write
		if err := json.NewEncoder(rw).Encode(b); err != nil {
			s.l.Error(fmt.Errorf("astiencoder: writing failed: %w", err))
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

func (s *Server) servePlaybackUnload() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Unload
		if err := s.p.unload(); err != nil {
			s.l.Error(fmt.Errorf("astiencoder: unloading playback failed: %w", err))
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Write
		if s.w != nil {
			if err := json.NewEncoder(rw).Encode(s.newServerWorkflow()); err != nil {
				s.l.Error(fmt.Errorf("astiencoder: writing failed: %w", err))
				rw.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
	})
}
