package astiencoder

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/asticode/go-astikit"
	"github.com/asticode/go-astiws"
	"github.com/gorilla/websocket"
	"github.com/julienschmidt/httprouter"
)

type Server struct {
	cs map[*astiws.Client]bool
	l  astikit.SeverityLogger
	m  *sync.Mutex // Locks cs
	w  *Workflow
	ws *astiws.Manager
}

type ServerOptions struct {
	Logger astikit.StdLogger
}

func NewServer(o ServerOptions) *Server {
	return &Server{
		cs: make(map[*astiws.Client]bool),
		l:  astikit.AdaptStdLogger(o.Logger),
		m:  &sync.Mutex{},
		ws: astiws.NewManager(astiws.ManagerConfiguration{MaxMessageSize: 1e6}, o.Logger),
	}
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
	// Add client
	s.m.Lock()
	s.cs[c] = true
	s.m.Unlock()

	// Add listeners
	c.AddListener(astiws.EventNameDisconnect, s.webSocketDisconnected)
	c.AddListener("ping", s.webSocketPing)
	return
}

func (s *Server) webSocketDisconnected(c *astiws.Client, eventName string, payload json.RawMessage) error {
	// Remove client
	s.m.Lock()
	delete(s.cs, c)
	s.m.Unlock()
	return nil
}

func (s *Server) webSocketPing(c *astiws.Client, eventName string, payload json.RawMessage) error {
	if err := c.ExtendConnection(); err != nil {
		s.l.Error(fmt.Errorf("astiencoder: extending ws connection failed: %w", err))
	}
	return nil
}

func (s *Server) sendWebSocket(eventName string, payload interface{}) {
	// Get clients
	var cs []*astiws.Client
	s.m.Lock()
	for c := range s.cs {
		cs = append(cs, c)
	}
	s.m.Unlock()

	// Loop through clients
	for _, c := range cs {
		if err := c.Write(eventName, payload); err != nil {
			s.l.Error(fmt.Errorf("astiencoder: writing event %s with payload %+v to websocket client %p failed: %w", eventName, payload, c, err))
		}
	}
}

func serverEventHandlerAdapter(eh *EventHandler, fn func(name string, payload interface{})) {
	// Register catch all handler
	eh.AddForAll(func(e Event) bool {
		// Get payload
		var p interface{}
		switch e.Name {
		case EventNameError:
			p = astikit.ErrorCause(e.Payload.(error))
		case EventNameNodeContinued, EventNameNodePaused, EventNameNodeStopped:
			p = e.Target.(Node).Metadata().Name
		case EventNameNodeStarted:
			p = newServerNode(e.Target.(Node))
		case EventNameStats:
			p = newServerStats(e)
		}

		// Custom
		fn(e.Name, p)
		return false
	})
}

func (s *Server) EventHandlerAdapter(eh *EventHandler) {
	serverEventHandlerAdapter(eh, s.sendWebSocket)
}

type ServerWorkflow struct {
	Name   string       `json:"name"`
	Nodes  []ServerNode `json:"nodes"`
	Status string       `json:"status"`
}

func newServerWorkflow(w *Workflow) (sw *ServerWorkflow) {
	// Create server workflow
	sw = &ServerWorkflow{
		Name:   w.Name(),
		Nodes:  []ServerNode{},
		Status: w.Status(),
	}

	// Discover nodes
	ns := make(map[string]ServerNode)
	for _, n := range w.Children() {
		discoverServerNode(n, ns)
	}

	// Add nodes
	for _, n := range ns {
		sw.Nodes = append(sw.Nodes, n)
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

func newServerStats(e Event) (ss []ServerStat) {
	for _, es := range e.Payload.([]EventStat) {
		ss = append(ss, newServerStat(es))
	}
	return
}

type ServerStat struct {
	Description string      `json:"description"`
	Label       string      `json:"label"`
	Name        string      `json:"name"`
	Target      string      `json:"target"`
	Unit        string      `json:"unit"`
	Value       interface{} `json:"value"`
}

func newServerStat(e EventStat) (s ServerStat) {
	s = ServerStat{
		Description: e.Description,
		Label:       e.Label,
		Name:        e.Name,
		Unit:        e.Unit,
		Value:       e.Value,
	}
	if n, ok := e.Target.(Node); ok {
		s.Target = n.Metadata().Name
	} else if w, ok := e.Target.(*Workflow); ok {
		s.Target = w.Name()
	}
	return
}

type ServerWelcome struct {
	Workflow *ServerWorkflow `json:"workflow,omitempty"`
}

func (s *Server) serveWelcome() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Create body
		var b ServerWelcome
		if s.w != nil {
			b.Workflow = newServerWorkflow(s.w)
		}

		// Write
		if err := json.NewEncoder(rw).Encode(b); err != nil {
			s.l.Error(fmt.Errorf("astiencoder: writing failed: %w", err))
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}
