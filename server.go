package astiencoder

import (
	"encoding/json"
	"mime/multipart"
	"net/http"
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

// Websocket event names
const (
	websocketEventNamePing = "ping"
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
			name = "/index"
		}

		// Check if template exists
		var code = http.StatusOK
		name += ".html"
		if _, ok := s.t.Template(name); !ok {
			code = http.StatusNotFound
			name = "/errors/404.html"
		}

		// Write header
		rw.WriteHeader(code)

		// Execute template
		tpl, _ := s.t.Template(name)
		if err := tpl.Execute(rw, nil); err != nil {
			astilog.Error(errors.Wrapf(err, "astiencoder: executing template %s failed", name))
			return
		}
	}
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
		// TODO Do stuff with the event
	}
}
