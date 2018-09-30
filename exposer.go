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
	Workflows []ExposedWorkflowBase `json:"workflows"`
}

// ExposedWorkflow represents an exposed workflow
type ExposedWorkflow struct {
	ExposedWorkflowBase
	Edges []ExposedWorkflowEdge `json:"edges"`
	Job   Job                   `json:"job"`
	Nodes []ExposedWorkflowNode `json:"nodes"`
}

func newExposedWorkflow(w *Workflow) (o ExposedWorkflow) {
	// Init
	o = ExposedWorkflow{
		ExposedWorkflowBase: newExposedWorkflowBase(w),
		Edges:               []ExposedWorkflowEdge{},
		Job:                 w.j,
		Nodes:               []ExposedWorkflowNode{},
	}

	// Loop through children
	for _, n := range w.bn.Children() {
		o.parseNode(n)
	}
	return
}

func (w *ExposedWorkflow) parseNode(p Node) {
	// Append node
	w.Nodes = append(w.Nodes, newExposedWorkflowNode(p))

	// Loop through children
	for _, c := range p.Children() {
		// Append edge
		w.Edges = append(w.Edges, newExposedWorkflowEdge(p, c))

		// Parse node
		w.parseNode(c)
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

func (e *exposer) encoder() (o ExposedEncoder) {
	e.e.m.Lock()
	defer e.e.m.Unlock()
	o = ExposedEncoder{
		Workflows: []ExposedWorkflowBase{},
	}
	for _, w := range e.e.ws {
		o.Workflows = append(o.Workflows, newExposedWorkflowBase(w))
	}
	return
}

func (e *exposer) addWorkflow(name string, j Job) (err error) {
	_, err = e.e.NewWorkflow(name, j)
	return
}

func (e *exposer) workflow(name string) (ew ExposedWorkflow, err error) {
	var w *Workflow
	if w, err = e.e.Workflow(name); err != nil {
		return
	}
	ew = newExposedWorkflow(w)
	return
}

func (e *exposer) continueWorkflow(name string) (err error) {
	// Retrieve workflow
	var w *Workflow
	if w, err = e.e.Workflow(name); err != nil {
		return
	}

	// Continue workflow
	w.Continue()
	return
}

func (e *exposer) pauseWorkflow(name string) (err error) {
	// Retrieve workflow
	var w *Workflow
	if w, err = e.e.Workflow(name); err != nil {
		return
	}

	// Pause workflow
	w.Pause()
	return
}

func (e *exposer) startWorkflow(name string) (err error) {
	// Retrieve workflow
	var w *Workflow
	if w, err = e.e.Workflow(name); err != nil {
		return
	}

	// Start workflow
	w.Start()
	return
}

func (e *exposer) continueNode(workflow, node string) (err error) {
	// Retrieve workflow
	var w *Workflow
	if w, err = e.e.Workflow(workflow); err != nil {
		return
	}

	// Retrieve node
	var n Node
	if n, err = w.Node(node); err != nil {
		return
	}

	// Continue node
	n.Continue()
	return
}

func (e *exposer) pauseNode(workflow, node string) (err error) {
	// Retrieve workflow
	var w *Workflow
	if w, err = e.e.Workflow(workflow); err != nil {
		return
	}

	// Retrieve node
	var n Node
	if n, err = w.Node(node); err != nil {
		return
	}

	// Pause node
	n.Pause()
	return
}

func (e *exposer) startNode(workflow, node string) (err error) {
	// Retrieve workflow
	var w *Workflow
	if w, err = e.e.Workflow(workflow); err != nil {
		return
	}

	// Retrieve node
	var n Node
	if n, err = w.Node(node); err != nil {
		return
	}

	// Check workflow status
	if w.Status() == StatusRunning {
		// Workflow is running, we only start the desired node
		w.startNode(n)
	} else {
		// Workflow is stopped, we start it as well as the desired node
		w.start(n)
	}
	return
}
