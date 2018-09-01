package astiencoder

type encoder struct {
	d *dispatcher
}

func newEncoder(d *dispatcher) *encoder {
	return &encoder{
		d: d,
	}
}
