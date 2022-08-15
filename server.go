package astiencoder

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/asticode/go-astikit"
)

type Server struct {
	l astikit.SeverityLogger
	n ServerNotifierFunc
	w *Workflow
}

type ServerNotifierFunc func(eventName string, payload interface{})

type ServerOptions struct {
	Logger       astikit.StdLogger
	NotifierFunc ServerNotifierFunc
}

func NewServer(o ServerOptions) *Server {
	return &Server{
		l: astikit.AdaptStdLogger(o.Logger),
		n: o.NotifierFunc,
	}
}

func (s *Server) SetWorkflow(w *Workflow) {
	s.w = w
}

func serverEventHandlerAdapter(eh *EventHandler, fn func(name string, payload interface{})) {
	// Register catch all handler
	eh.AddForAll(func(e Event) bool {
		// Get payload
		var p interface{}
		switch e.Name {
		case EventNameError:
			p = astikit.ErrorCause(e.Payload.(error))
		case EventNameNodeChildAdded, EventNameNodeChildRemoved:
			if e.Payload.(Node).Status() == StatusCreated || e.Target.(Node).Status() == StatusCreated {
				return false
			}
			p = ServerChildUpdate{
				Child:  e.Payload.(Node).Metadata().Name,
				Parent: e.Target.(Node).Metadata().Name,
			}
		case EventNameNodeClosed, EventNameNodeContinued, EventNameNodePaused, EventNameNodeStopped:
			p = e.Target.(Node).Metadata().Name
		case EventNameNodeStarted:
			p = newServerNode(e.Target.(Node))
		case EventNameStats:
			p = newServerStats(e)
		default:
			return false
		}

		// Custom
		fn(e.Name, p)
		return false
	})
}

func (s *Server) EventHandlerAdapter(eh *EventHandler) {
	if s.n == nil {
		return
	}
	serverEventHandlerAdapter(eh, s.n)
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
	Closed      bool     `json:"closed"`
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
		Closed:      n.IsClosed(),
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

type ServerChildUpdate struct {
	Child  string `json:"child"`
	Parent string `json:"parent"`
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

func (s *Server) ServeWelcome() http.Handler {
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
