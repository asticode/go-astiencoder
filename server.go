package astiencoder

import (
	"net/http"

	"github.com/asticode/go-astitools/http"
	"github.com/julienschmidt/httprouter"
)

type server struct {
	c  ConfigurationServer
	fn func(addr string, h http.Handler)
}

func newServer(c ConfigurationServer, fn func(addr string, h http.Handler)) *server {
	return &server{
		c:  c,
		fn: fn,
	}
}

func (s *server) start() {
	s.fn(s.c.Addr, s.handler())
}

func (s *server) handler() http.Handler {
	// Init router
	var r = httprouter.New()

	// Routes
	r.GET("/ok", s.handleOK())

	// Chain middlewares
	var h = astihttp.ChainMiddlewares(r, astihttp.MiddlewareBasicAuth(s.c.Username, s.c.Password))
	return h
}

func (s *server) handleOK() httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {}
}
