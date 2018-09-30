package astiencoder

import (
	"encoding/json"
	"fmt"
	"mime/multipart"
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

type server struct {
	c ConfigurationServer
	e *exposer
	m *astiws.Manager
	t *astitemplate.Templater
}

func newServer(c ConfigurationServer, e *exposer) (s *server, err error) {
	// Create server
	s = &server{
		c: c,
		e: e,
		m: astiws.NewManager(astiws.ManagerConfiguration{MaxMessageSize: 8192}),
	}

	// Create templater
	if s.t, err = astitemplate.NewTemplater(filepath.Join(c.PathWeb, "templates"), filepath.Join(c.PathWeb, "layouts"), ".html"); err != nil {
		err = errors.Wrap(err, "astiencoder: creating templater failed")
		return
	}
	return
}

func (s *server) handler() http.Handler {
	// Init router
	var r = httprouter.New()

	// Web
	r.GET("/", s.handleHomepage())
	r.ServeFiles("/static/*filepath", http.Dir(filepath.Join(s.c.PathWeb, "static")))
	r.GET("/web/*page", s.handleWeb())

	// Websocket
	r.GET("/websocket", s.handleWebsocket())

	// API
	r.GET("/api/ok", s.handleOK())
	r.GET("/api/references", s.handleReferences())
	r.GET("/api/encoder", s.handleEncoder())
	r.GET("/api/encoder/stop", s.handleEncoderStop())
	r.POST("/api/workflows", s.handleAddWorkflow())
	r.GET("/api/workflows/:workflow", s.handleWorkflow())
	r.GET("/api/workflows/:workflow/nodes/:node/start", s.handleNodeStart())
	r.GET("/api/workflows/:workflow/nodes/:node/stop", s.handleNodeStop())
	r.GET("/api/workflows/:workflow/start", s.handleWorkflowStart())
	r.GET("/api/workflows/:workflow/stop", s.handleWorkflowStop())

	// Chain middlewares
	var h = astihttp.ChainMiddlewares(r, astihttp.MiddlewareBasicAuth(s.c.Username, s.c.Password))
	h = astihttp.ChainMiddlewaresWithPrefix(h, []string{"/web/"}, astihttp.MiddlewareContentType("text/html; charset=UTF-8"))
	h = astihttp.ChainMiddlewaresWithPrefix(h, []string{"/api/"}, astihttp.MiddlewareContentType("application/json"))
	return h
}

func (s *server) handleHomepage() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		http.Redirect(rw, r, "/web/", http.StatusTemporaryRedirect)
	}
}

func (s *server) handleWeb() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// Get page
		name := p.ByName("page")
		if len(name) == 0 || name == "/" {
			if e := s.e.encoder(); len(e.Workflows) == 1 {
				http.Redirect(rw, r, "/web/workflow?name="+url.QueryEscape(e.Workflows[0].Name), http.StatusTemporaryRedirect)
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

func (s *server) templateData(name string, r *http.Request, code *int) (d interface{}) {
	var err error
	switch name {
	case "/workflow.html":
		if d, err = s.e.workflow(r.URL.Query().Get("name")); err != nil {
			if err == ErrWorkflowNotFound {
				*code = http.StatusNotFound
			} else {
				*code = http.StatusInternalServerError
			}
			return
		}
	}
	return
}

// APIError represents an API error.
type APIError struct {
	Message string `json:"message"`
}

// APIRedirect represents an API redirect
type APIRedirect struct {
	Redirect string `json:"redirect"`
}

// APIReferences represents the API references.
type APIReferences struct {
	WsPingPeriod time.Duration `json:"ws_ping_period"`
}

func (s *server) writeJSONData(rw http.ResponseWriter, data interface{}) {
	if err := json.NewEncoder(rw).Encode(data); err != nil {
		s.writeJSONError(rw, http.StatusInternalServerError, errors.Wrap(err, "astiencoder: json encoding failed"))
		return
	}
}

func (s *server) writeJSONError(rw http.ResponseWriter, code int, err error) {
	rw.WriteHeader(code)
	astilog.Error(err)
	if err := json.NewEncoder(rw).Encode(APIError{Message: errors.Cause(err).Error()}); err != nil {
		astilog.Error(errors.Wrap(err, "astiencoder: json encoding failed"))
	}
}

func (s *server) handleOK() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		rw.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) handleReferences() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		s.writeJSONData(rw, APIReferences{
			WsPingPeriod: astiws.PingPeriod,
		})
	}
}

func (s *server) handleEncoder() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) { s.writeJSONData(rw, s.e.encoder()) }
}

func (s *server) handleEncoderStop() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) { s.e.stopEncoder() }
}

func (s *server) handleAddWorkflow() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// Get name
		var name string
		if name = r.FormValue("name"); len(name) == 0 {
			s.writeJSONError(rw, http.StatusBadRequest, errors.New("astiencoder: name is mandatory"))
			return
		}

		// Parse job
		var f multipart.File
		var err error
		if f, _, err = r.FormFile("job"); err != nil {
			if err != http.ErrMissingFile {
				s.writeJSONError(rw, http.StatusInternalServerError, errors.Wrap(err, "astiencoder: getting form file failed"))
			} else {
				s.writeJSONError(rw, http.StatusBadRequest, errors.New("astiencoder: job is mandatory"))
			}
			return
		}

		// Unmarshal
		var j Job
		if err = json.NewDecoder(f).Decode(&j); err != nil {
			s.writeJSONError(rw, http.StatusBadRequest, errors.Wrap(err, "astiencoder: unmarshaling job failed"))
			return
		}

		// Add workflow
		if err := s.e.addWorkflow(name, j); err != nil {
			s.writeJSONError(rw, http.StatusBadRequest, errors.Wrapf(err, "astiencoder: adding workflow %s failed", name))
			return
		}
	}
}

func (s *server) handleWorkflow() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// Get workflow
		w, err := s.e.workflow(p.ByName("workflow"))
		if err != nil {
			if err == ErrWorkflowNotFound {
				s.writeJSONError(rw, http.StatusNotFound, fmt.Errorf("astiencoder: workflow %s doesn't exist", p.ByName("workflow")))
			} else {
				s.writeJSONError(rw, http.StatusInternalServerError, errors.Wrapf(err, "astiencoder: fetching workflow %s failed", p.ByName("workflow")))
			}
			return
		}

		// Write
		if err := json.NewEncoder(rw).Encode(w); err != nil {
			s.writeJSONError(rw, http.StatusInternalServerError, errors.Wrap(err, "astiencoder: writing failed"))
			return
		}
	}
}

func (s *server) handleWorkflowStart() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// Start workflow
		if err := s.e.startWorkflow(p.ByName("workflow")); err != nil {
			if err == ErrWorkflowNotFound {
				s.writeJSONError(rw, http.StatusNotFound, fmt.Errorf("astiencoder: workflow %s doesn't exist", p.ByName("workflow")))
			} else {
				s.writeJSONError(rw, http.StatusInternalServerError, errors.Wrapf(err, "astiencode: starting workflow %s failed", p.ByName("workflow")))
			}
			return
		}
	}
}

func (s *server) handleWorkflowStop() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// Stop workflow
		if err := s.e.stopWorkflow(p.ByName("workflow")); err != nil {
			if err == ErrWorkflowNotFound {
				s.writeJSONError(rw, http.StatusNotFound, fmt.Errorf("astiencoder: workflow %s doesn't exist", p.ByName("workflow")))
			} else {
				s.writeJSONError(rw, http.StatusInternalServerError, errors.Wrapf(err, "astiencode: stopping workflow %s failed", p.ByName("workflow")))
			}
			return
		}
	}
}

func (s *server) handleNodeStart() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// Start node
		if err := s.e.startNode(p.ByName("workflow"), p.ByName("node")); err != nil {
			if err == ErrWorkflowNotFound {
				s.writeJSONError(rw, http.StatusNotFound, fmt.Errorf("astiencoder: workflow %s doesn't exist", p.ByName("workflow")))
			} else if err == ErrNodeNotFound {
				s.writeJSONError(rw, http.StatusNotFound, fmt.Errorf("astiencoder: node %s doesn't exist", p.ByName("node")))
			} else {
				s.writeJSONError(rw, http.StatusInternalServerError, errors.Wrapf(err, "astiencode: starting node %s of workflow %s failed", p.ByName("node"), p.ByName("workflow")))
			}
			return
		}
	}
}

func (s *server) handleNodeStop() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// Stop node
		if err := s.e.stopNode(p.ByName("workflow"), p.ByName("node")); err != nil {
			if err == ErrWorkflowNotFound {
				s.writeJSONError(rw, http.StatusNotFound, fmt.Errorf("astiencoder: workflow %s doesn't exist", p.ByName("workflow")))
			} else if err == ErrNodeNotFound {
				s.writeJSONError(rw, http.StatusNotFound, fmt.Errorf("astiencoder: node %s doesn't exist", p.ByName("node")))
			} else {
				s.writeJSONError(rw, http.StatusInternalServerError, errors.Wrapf(err, "astiencode: stopping node %s of workflow %s failed", p.ByName("node"), p.ByName("workflow")))
			}
			return
		}
	}
}

func (s *server) handleWebsocket() httprouter.Handle {
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

func (s *server) adaptWebsocketClient(c *astiws.Client) {
	// Register client
	s.m.AutoRegisterClient(c)

	// Add listeners
	c.AddListener(astiws.EventNameDisconnect, s.handleWebsocketDisconnected)
	c.AddListener(websocketEventNamePing, s.handleWebsocketPing)
}

func (s *server) handleWebsocketDisconnected(c *astiws.Client, eventName string, payload json.RawMessage) error {
	s.m.UnregisterClient(c)
	return nil
}

func (s *server) handleWebsocketPing(c *astiws.Client, eventName string, payload json.RawMessage) error {
	if err := c.HandlePing(); err != nil {
		astilog.Error(errors.Wrap(err, "astiencoder: handling ping failed"))
	}
	return nil
}

func (s *server) handleEvent() (isBlocking bool, fn func(e Event)) {
	return false, func(e Event) {
		switch e.Name {
		case EventNameError:
			s.sendEventToWebsocket(e.Name, errors.Cause(e.Payload.(error)))
		default:
			s.sendEventToWebsocket(e.Name, e.Payload)
		}
	}
}

func (s *server) sendEventToWebsocket(eventName string, payload interface{}) {
	s.m.Loop(func(_ interface{}, c *astiws.Client) {
		if err := c.Write(eventName, payload); err != nil {
			astilog.Error(errors.Wrapf(err, "astiencoder: writing event %s with payload %+v to websocket client %p failed", eventName, payload, c))
			return
		}
	})
}
