package astiencoder

// Job represents a job
type Job struct {
	Inputs     map[string]JobInput     `json:"inputs"`
	Operations map[string]JobOperation `json:"operations"`
	Outputs    map[string]JobOutput    `json:"outputs"`
}

// JobInput represents a job input
type JobInput struct {
	URL string `json:"url"`
}

// JobOutput represents a job output
type JobOutput struct {
	URL string `json:"url"`
}

// Job operation types
const (
	JobOperationTypeRemux = "remux"
)

// JobOperation represents a job operation
type JobOperation struct {
	Inputs  []JobOperationInput  `json:"inputs"`
	Outputs []JobOperationOutput `json:"outputs"`
	Type    string               `json:"type"`
}

// JobOperationInput represents a job operation input
type JobOperationInput struct {
	Name string `json:"name"`
	PID  string `json:"pid"`
}

// JobOperationOutput represents a job operation output
type JobOperationOutput struct {
	Name string `json:"name"`
	PID  string `json:"pid"`
}
