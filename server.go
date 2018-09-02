package astiencoder

import (
	"net/http"

	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astitools/http"
	"github.com/asticode/go-astiws"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

type server struct {
	c ConfigurationServer
	e *eventEmitter
	m *astiws.Manager
}

func newServer(c ConfigurationServer, e *eventEmitter) *server {
	return &server{
		c: c,
		e: e,
		m: astiws.NewManager(astiws.ManagerConfiguration{MaxMessageSize: 8192}),
	}
}

func (s *server) handler() http.Handler {
	// Init router
	var r = httprouter.New()

	// Routes
	r.GET("/", s.handleHomepage())
	r.GET("/ok", s.handleOK())
	r.GET("/websocket", s.handleWebsocket())
	r.ServeFiles("/web/*filepath", http.Dir(s.c.PathWeb))

	// Chain middlewares
	var h = astihttp.ChainMiddlewares(r, astihttp.MiddlewareBasicAuth(s.c.Username, s.c.Password))
	return h
}

func (s *server) handleHomepage() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		http.Redirect(rw, r, "/web/index.html", http.StatusTemporaryRedirect)
	}
}

func (s *server) handleOK() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {}
}

func (s *server) handleWebsocket() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if err := s.m.ServeHTTP(rw, r, s.websocketClientAdapter); err != nil {
			astilog.Error(errors.Wrap(err, "astiencoder: serving websocket failed"))
		}
	}
}

func (s *server) websocketClientAdapter(c *astiws.Client) {
	s.m.AutoRegisterClient(c)
	// TODO Do stuff with the client
}

func (s *server) handleEvent(e Event) {
	// TODO Do stuff with the event
}
