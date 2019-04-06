package astiencoder

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astitools/http"
	"github.com/asticode/go-astitools/template"
	"github.com/asticode/go-astiws"
	"github.com/gorilla/websocket"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

// ExposedError represents an exposed error.
type ExposedError struct {
	Message string `json:"message"`
}

// ExposedReferences represents the exposed references.
type ExposedReferences struct {
	WsPingPeriod time.Duration `json:"ws_ping_period"`
}

// ExposedWorkflow represents an exposed workflow
type ExposedWorkflow struct {
	ExposedWorkflowBase
	Edges []ExposedWorkflowEdge `json:"edges"`
	Nodes []ExposedWorkflowNode `json:"nodes"`
}

func newExposedWorkflow(w *Workflow) (o ExposedWorkflow) {
	// Init
	o = ExposedWorkflow{
		ExposedWorkflowBase: newExposedWorkflowBase(w),
		Edges:               []ExposedWorkflowEdge{},
		Nodes:               []ExposedWorkflowNode{},
	}

	// Loop through children
	var processedEdges = make(map[string]bool)
	for _, n := range w.bn.Children() {
		o.parseNode(n, processedEdges)
	}
	return
}

func (w *ExposedWorkflow) parseNode(p Node, processedEdges map[string]bool) {
	// Append node
	w.Nodes = append(w.Nodes, newExposedWorkflowNode(p))

	// Loop through children
	for _, c := range p.Children() {
		// Append edge
		k := fmt.Sprintf("%s --> %s", p.Metadata().Name, c.Metadata().Name)
		if _, ok := processedEdges[k]; !ok {
			w.Edges = append(w.Edges, newExposedWorkflowEdge(p, c))
			processedEdges[k] = true
		}

		// Parse node
		w.parseNode(c, processedEdges)
	}
}

// ExposedWorkflowBase represents a base exposed encoder workflow
type ExposedWorkflowBase struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func newExposedWorkflowBase(w *Workflow) ExposedWorkflowBase {
	return ExposedWorkflowBase{
		Name:   w.name,
		Status: w.Status(),
	}
}

// ExposedWorkflowEdge represents an exposed workflow edge
type ExposedWorkflowEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func newExposedWorkflowEdge(parent, child Node) ExposedWorkflowEdge {
	return ExposedWorkflowEdge{
		From: parent.Metadata().Name,
		To:   child.Metadata().Name,
	}
}

// ExposedWorkflowNode represents an exposed workflow node
type ExposedWorkflowNode struct {
	Description string                `json:"description"`
	Label       string                `json:"label"`
	Name        string                `json:"name"`
	Stats       []ExposedStatMetadata `json:"stats"`
	Status      string                `json:"status"`
}

// ExposedStatMetadata represents exposed stat metadata
type ExposedStatMetadata struct {
	Description string `json:"description"`
	Label       string `json:"label"`
	Unit        string `json:"unit"`
}

func newExposedWorkflowNode(n Node) (w ExposedWorkflowNode) {
	w = ExposedWorkflowNode{
		Description: n.Metadata().Description,
		Label:       n.Metadata().Label,
		Name:        n.Metadata().Name,
		Stats:       []ExposedStatMetadata{},
		Status:      n.Status(),
	}
	if s := n.Stater(); s != nil {
		for _, v := range s.StatsMetadata() {
			w.Stats = append(w.Stats, ExposedStatMetadata{
				Description: v.Description,
				Label:       v.Label,
				Unit:        v.Unit,
			})
		}
	}
	return
}

type workflowPoolServer struct {
	m       *astiws.Manager
	pathWeb string
	t       *astitemplate.Templater
	wp      *WorkflowPool
}

func newWorkflowPoolServer(wp *WorkflowPool, pathWeb string) (s *workflowPoolServer, err error) {
	// Create server
	s = &workflowPoolServer{
		m:       astiws.NewManager(astiws.ManagerConfiguration{MaxMessageSize: 8192}),
		pathWeb: pathWeb,
		wp:      wp,
	}

	// Create templater
	if s.t, err = astitemplate.NewTemplater(filepath.Join(pathWeb, "templates"), filepath.Join(pathWeb, "layouts"), ".html"); err != nil {
		err = errors.Wrap(err, "astiencoder: creating templater failed")
		return
	}
	return
}

func (s *workflowPoolServer) handler() http.Handler {
	// Init router
	var r = httprouter.New()

	// Web
	r.GET("/", s.handleHomepage())
	r.ServeFiles("/static/*filepath", http.Dir(filepath.Join(s.pathWeb, "static")))
	r.GET("/web/*page", s.handleWeb())

	// Websocket
	r.GET("/websocket", s.handleWebsocket())

	// API
	r.GET("/api/ok", s.handleOK())
	r.GET("/api/references", s.handleReferences())
	r.GET("/api/workflows", s.handleWorkflows())
	r.GET("/api/workflows/:workflow", s.handleWorkflow())
	r.GET("/api/workflows/:workflow/nodes/:node/continue", s.handleNodeContinue())
	r.GET("/api/workflows/:workflow/nodes/:node/pause", s.handleNodePause())
	r.GET("/api/workflows/:workflow/nodes/:node/start", s.handleNodeStart())
	r.GET("/api/workflows/:workflow/continue", s.handleWorkflowContinue())
	r.GET("/api/workflows/:workflow/pause", s.handleWorkflowPause())
	r.GET("/api/workflows/:workflow/start", s.handleWorkflowStart())

	// Chain middlewares
	var h = astihttp.ChainMiddlewaresWithPrefix(r, []string{"/web/"}, astihttp.MiddlewareContentType("text/html; charset=UTF-8"))
	h = astihttp.ChainMiddlewaresWithPrefix(h, []string{"/api/"}, astihttp.MiddlewareContentType("application/json"))
	return h
}

func (s *workflowPoolServer) handleHomepage() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		http.Redirect(rw, r, "/web/", http.StatusTemporaryRedirect)
	}
}

func (s *workflowPoolServer) handleWeb() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// Get page
		name := p.ByName("page")
		if len(name) == 0 || name == "/" {
			if ws := s.wp.Workflows(); len(ws) == 1 {
				http.Redirect(rw, r, "/web/workflow?name="+url.QueryEscape(ws[0].name), http.StatusTemporaryRedirect)
				return
			}
			name = "/index"
		}

		// Check if template exists
		var code = http.StatusOK
		name += ".html"
		if _, ok := s.t.Template(name); !ok {
			code = http.StatusNotFound
		}

		// Get data
		d := s.templateData(name, r, &code)

		// Handle errors
		if code != http.StatusOK {
			name = fmt.Sprintf("/errors/%d.html", code)
		}

		// Write header
		rw.WriteHeader(code)

		// Execute template
		tpl, _ := s.t.Template(name)
		if err := tpl.Execute(rw, d); err != nil {
			astilog.Error(errors.Wrapf(err, "astiencoder: executing template %s with data %+v failed", name, d))
			return
		}
	}
}

func (s *workflowPoolServer) templateData(name string, r *http.Request, code *int) (d interface{}) {
	var err error
	switch name {
	case "/workflow.html":
		// Retrieve workflow
		var w *Workflow
		if w, err = s.wp.Workflow(r.URL.Query().Get("name")); err != nil {
			if err == ErrWorkflowNotFound {
				*code = http.StatusNotFound
			} else {
				*code = http.StatusInternalServerError
			}
			return
		}

		// Create exposed workflow
		d = newExposedWorkflow(w)
	}
	return
}

func (s *workflowPoolServer) writeJSONData(rw http.ResponseWriter, data interface{}) {
	if err := json.NewEncoder(rw).Encode(data); err != nil {
		WriteJSONError(rw, http.StatusInternalServerError, errors.Wrap(err, "astiencoder: json encoding failed"))
		return
	}
}

// WriteJSONError writes a JSON error
func WriteJSONError(rw http.ResponseWriter, code int, err error) {
	rw.WriteHeader(code)
	astilog.Error(err)
	if err := json.NewEncoder(rw).Encode(ExposedError{Message: errors.Cause(err).Error()}); err != nil {
		astilog.Error(errors.Wrap(err, "astiencoder: json encoding failed"))
	}
}

func (s *workflowPoolServer) handleOK() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		rw.WriteHeader(http.StatusNoContent)
	}
}

func (s *workflowPoolServer) handleReferences() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		s.writeJSONData(rw, ExposedReferences{
			WsPingPeriod: astiws.PingPeriod,
		})
	}
}

func (s *workflowPoolServer) handleWorkflows() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		ws := []ExposedWorkflow{}
		for _, w := range s.wp.Workflows() {
			ws = append(ws, newExposedWorkflow(w))
		}
		s.writeJSONData(rw, ws)
	}
}

func (s *workflowPoolServer) handleWorkflowAction(fn func(w *Workflow, rw http.ResponseWriter, p httprouter.Params)) httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// Get workflow
		w, err := s.wp.Workflow(p.ByName("workflow"))
		if err != nil {
			if err == ErrWorkflowNotFound {
				WriteJSONError(rw, http.StatusNotFound, fmt.Errorf("astiencoder: workflow %s doesn't exist", p.ByName("workflow")))
			} else {
				WriteJSONError(rw, http.StatusInternalServerError, errors.Wrapf(err, "astiencoder: fetching workflow %s failed", p.ByName("workflow")))
			}
			return
		}

		// Custom
		fn(w, rw, p)
	}
}

func (s *workflowPoolServer) handleWorkflow() httprouter.Handle {
	return s.handleWorkflowAction(func(w *Workflow, rw http.ResponseWriter, p httprouter.Params) {
		if err := json.NewEncoder(rw).Encode(newExposedWorkflow(w)); err != nil {
			WriteJSONError(rw, http.StatusInternalServerError, errors.Wrap(err, "astiencoder: writing failed"))
			return
		}
	})
}

func (s *workflowPoolServer) handleWorkflowContinue() httprouter.Handle {
	return s.handleWorkflowAction(func(w *Workflow, rw http.ResponseWriter, p httprouter.Params) { w.Continue() })
}

func (s *workflowPoolServer) handleWorkflowPause() httprouter.Handle {
	return s.handleWorkflowAction(func(w *Workflow, rw http.ResponseWriter, p httprouter.Params) { w.Pause() })
}

func (s *workflowPoolServer) handleWorkflowStart() httprouter.Handle {
	return s.handleWorkflowAction(func(w *Workflow, rw http.ResponseWriter, p httprouter.Params) { w.Start() })
}

func (s *workflowPoolServer) handleNodeAction(fn func(w *Workflow, n Node)) httprouter.Handle {
	return s.handleWorkflowAction(func(w *Workflow, rw http.ResponseWriter, p httprouter.Params) {
		// Get node
		n, ok := w.indexedNodes()[p.ByName("node")]
		if !ok {
			WriteJSONError(rw, http.StatusNotFound, fmt.Errorf("astiencoder: node %s doesn't exist", p.ByName("node")))
			return
		}

		// Custom
		fn(w, n)
	})
}

func (s *workflowPoolServer) handleNodeContinue() httprouter.Handle {
	return s.handleNodeAction(func(w *Workflow, n Node) { n.Continue() })
}

func (s *workflowPoolServer) handleNodePause() httprouter.Handle {
	return s.handleNodeAction(func(w *Workflow, n Node) { n.Pause() })
}

func (s *workflowPoolServer) handleNodeStart() httprouter.Handle {
	return s.handleNodeAction(func(w *Workflow, n Node) {
		if w.Status() == StatusRunning {
			w.StartNodes(n)
		} else {
			w.start([]Node{n}, WorkflowStartOptions{})
		}
	})
}

func (s *workflowPoolServer) handleWebsocket() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if err := s.m.ServeHTTP(rw, r, s.adaptWebsocketClient); err != nil {
			if v, ok := errors.Cause(err).(*websocket.CloseError); !ok || (v.Code != websocket.CloseNoStatusReceived && v.Code != websocket.CloseNormalClosure) {
				astilog.Error(errors.Wrap(err, "astiencoder: handling websocket failed"))
			}
			return
		}
	}
}

const (
	websocketEventNamePing = "ping"
)

func (s *workflowPoolServer) adaptWebsocketClient(c *astiws.Client) (err error) {
	// Register client
	s.m.AutoRegisterClient(c)

	// Add listeners
	c.AddListener(astiws.EventNameDisconnect, s.handleWebsocketDisconnected)
	c.AddListener(websocketEventNamePing, s.handleWebsocketPing)
	return
}

func (s *workflowPoolServer) handleWebsocketDisconnected(c *astiws.Client, eventName string, payload json.RawMessage) error {
	s.m.UnregisterClient(c)
	return nil
}

func (s *workflowPoolServer) handleWebsocketPing(c *astiws.Client, eventName string, payload json.RawMessage) error {
	if err := c.ExtendConnection(); err != nil {
		astilog.Error(errors.Wrap(err, "astiencoder: extending ws connection failed"))
	}
	return nil
}

// ExposedStats represents exposed stats
type ExposedStats struct {
	Name  string        `json:"name"`
	Stats []ExposedStat `json:"stats"`
}

// ExposedStat represents an exposed stat
type ExposedStat struct {
	Description string      `json:"description"`
	Label       string      `json:"label"`
	Unit        string      `json:"unit"`
	Value       interface{} `json:"value"`
}

// HandleEvent implements the EventHandler interface
func (s *workflowPoolServer) adaptEventHandler(eh *EventHandler) {
	eh.AddForAll(func(e Event) bool {
		n := e.Name
		var p interface{}
		switch e.Name {
		case EventNameError:
			p = errors.Cause(e.Payload.(error))
		case EventNameWorkflowContinued, EventNameWorkflowPaused, EventNameWorkflowStarted, EventNameWorkflowStopped:
			p = e.Target.(*Workflow).Name()
		case EventNameNodeStats, EventNameWorkflowStats:
			np := ExposedStats{}
			if e.Name == EventNameNodeStats {
				np.Name = e.Target.(Node).Metadata().Name
			} else {
				np.Name = e.Target.(*Workflow).Name()
			}
			for _, s := range e.Payload.([]EventStat) {
				np.Stats = append(np.Stats, ExposedStat{
					Description: s.Description,
					Label:       s.Label,
					Unit:        s.Unit,
					Value:       s.Value,
				})
			}
			n = "stats"
			p = np
		case EventNameNodeContinued, EventNameNodePaused, EventNameNodeStarted, EventNameNodeStopped:
			p = e.Target.(Node).Metadata().Name
		}
		s.sendEventToWebsocket(n, p)
		return false
	})
}

func (s *workflowPoolServer) sendEventToWebsocket(eventName string, payload interface{}) {
	s.m.Loop(func(_ interface{}, c *astiws.Client) {
		if err := c.Write(eventName, payload); err != nil {
			astilog.Error(errors.Wrapf(err, "astiencoder: writing event %s with payload %+v to websocket client %p failed", eventName, payload, c))
			return
		}
	})
}
