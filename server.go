package astiencoder

import (
	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astiws"
	"github.com/pkg/errors"
	"net/http"

	"github.com/asticode/go-astitools/http"
	"github.com/julienschmidt/httprouter"
)

type server struct {
	c ConfigurationServer
	d *dispatcher
	m *astiws.Manager
}

func newServer(c ConfigurationServer, d *dispatcher, m *astiws.Manager) *server {
	return &server{
		c: c,
		d: d,
		m: m,
	}
}

func (s *server) start(fn func(addr string, h http.Handler)) {
	fn(s.c.Addr, s.handler())
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
