package astiencoder

type dispatcher struct {
	e *encoder
	s *server
}

func newDispatcher() *dispatcher {
	return &dispatcher{}
}

func (d *dispatcher) init(e *encoder, s *server) {
	d.e = e
	d.s = s
}

type event struct {
	name string
}

func (d *dispatcher) dispatch(e event) (err error) {
	return
}
