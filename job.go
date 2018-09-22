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

// Job output types
const (
	// The packet data is dumped directly somewhere without any mux
	JobOutputTypePktDump = "pkt_dump"
)

// JobOutput represents a job output
type JobOutput struct {
	// Default is "default". Possible values are "default" and "pkt_dump"
	Type string `json:"type"`
	URL  string `json:"url"`
}

// Job operation formats
const (
	JobOperationFormatJpeg = "jpeg"
)

// Job operation types
const (
	JobOperationTypeRemux = "remux"
)

// JobOperation represents a job operation
type JobOperation struct {
	// Default is "auto". Possible values are "auto" and "jpeg"
	Format string `json:"format"`
	// Frame rate is a per-operation value since we may have different frame rate operations for a similar output
	FrameRate float64              `json:"frame_rate"`
	Inputs    []JobOperationInput  `json:"inputs"`
	Outputs   []JobOperationOutput `json:"outputs"`
	// Default is "default". Possible values are "default" and "remux"
	Type string `json:"type"`
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
