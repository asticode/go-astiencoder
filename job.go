package astiencoder

import "github.com/asticode/go-astitools/float"

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
	// The packet data is dumped directly to the url without any mux
	JobOutputTypePktDump = "pkt_dump"
)

// JobOutput represents a job output
type JobOutput struct {
	// Possible values are "default" and "pkt_dump"
	Type string `json:"type,omitempty"`
	URL  string `json:"url"`
}

// Job operation codecs
const (
	JobOperationCodecCopy = "copy"
)

// JobOperation represents a job operation
// This can usually be compared to an encoding
// We need to refrain from using a dict here as we want to parse each and every attribute in order to provide detailed
// processing
type JobOperation struct {
	// Possible values are "copy" and all libav codec names.
	Codec string `json:"codec,omitempty"`
	// Frame rate is a per-operation value since we may have different frame rate operations for a similar output
	FrameRate   *astifloat.Rational  `json:"frame_rate,omitempty"`
	Height      *int                 `json:"height,omitempty"`
	Inputs      []JobOperationInput  `json:"inputs"`
	Outputs     []JobOperationOutput `json:"outputs"`
	PixelFormat string               `json:"pixel_format,omitempty"`
	// Since frame rate is a per-operation value, time base is as well
	TimeBase *astifloat.Rational `json:"time_base,omitempty"`
	Width    *int                `json:"width,omitempty"`
}

// JobOperationInput represents a job operation input
// TODO Add start, end and duration (use seek?)
type JobOperationInput struct {
	// Possible values are "audio", "subtitle" and "video"
	MediaType string `json:"media_type,omitempty"`
	Name      string `json:"name"`
	PID       *int   `json:"pid,omitempty"`
}

// JobOperationOutput represents a job operation output
type JobOperationOutput struct {
	Name string `json:"name"`
	PID  *int   `json:"pid,omitempty"`
}
