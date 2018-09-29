package astiencoder

type exposer struct {
	e *Encoder
}

func newExposer(e *Encoder) *exposer {
	return &exposer{e: e}
}

func (e *exposer) stopEncoder() {
	e.e.Stop()
}

// ExposedEncoder represents an exposed encoder.
type ExposedEncoder struct {
	Workflows []ExposedWorkflow `json:"workflows"`
}

// ExposedEncoderWorkflow represents an exposed encoder workflow
type ExposedWorkflow struct {
	Job    Job    `json:"job"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (e *exposer) encoder() (o ExposedEncoder) {
	e.e.m.Lock()
	defer e.e.m.Unlock()
	o = ExposedEncoder{
		Workflows: []ExposedWorkflow{},
	}
	for _, w := range e.e.ws {
		o.Workflows = append(o.Workflows, ExposedWorkflow{
			Job:    w.j,
			Name:   w.name,
			Status: w.status(),
		})
	}
	return
}

func (e *exposer) addWorkflow(name string, j Job) (err error) {
	_, err = e.e.NewWorkflow(name, j)
	return
}
