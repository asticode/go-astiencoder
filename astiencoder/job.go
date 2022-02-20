package main

import (
	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
)

// Job represents a job
type Job struct {
	Inputs     map[string]JobInput     `json:"inputs"`
	Operations map[string]JobOperation `json:"operations"`
	Outputs    map[string]JobOutput    `json:"outputs"`
}

// JobInput represents a job input
type JobInput struct {
	Dict        string `json:"dict"`
	EmulateRate bool   `json:"emulate_rate"`
	URL         string `json:"url"`
}

// Job output types
const (
	// The packet data is dumped directly to the url without any mux
	JobOutputTypePktDump = "pkt_dump"
)

// JobOutput represents a job output
type JobOutput struct {
	Format string `json:"format,omitempty"`
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
// Refrain from indicating all options in the dict and use other attributes instead
type JobOperation struct {
	BitRate *int64 `json:"bit_rate,omitempty"`
	// Possible values are "copy" and all libav codec names.
	Codec string `json:"codec,omitempty"`
	Dict  string `json:"dict,omitempty"`
	// Frame rate is a per-operation value since we may have different frame rate operations for a similar output
	FrameRate   *astikit.Rational    `json:"frame_rate,omitempty"`
	GopSize     *int                 `json:"gop_size,omitempty"`
	Height      *int                 `json:"height,omitempty"`
	Inputs      []JobOperationInput  `json:"inputs"`
	Outputs     []JobOperationOutput `json:"outputs"`
	ThreadCount *int                 `json:"thread_count,omitempty"`
	// Since frame rate is a per-operation value, time base is as well
	TimeBase *astikit.Rational `json:"time_base,omitempty"`
	Width    *int              `json:"width,omitempty"`
}

type JobMediaType astiav.MediaType

func (t *JobMediaType) UnmarshalText(b []byte) error {
	switch string(b) {
	case "audio":
		*t = JobMediaType(astiav.MediaTypeAudio)
	case "subtitle":
		*t = JobMediaType(astiav.MediaTypeSubtitle)
	case "video":
		*t = JobMediaType(astiav.MediaTypeVideo)
	default:
		*t = JobMediaType(astiav.MediaTypeUnknown)
	}
	return nil
}

func (t JobMediaType) MediaType() astiav.MediaType {
	return astiav.MediaType(t)
}

// JobOperationInput represents a job operation input
// TODO Add start, end and duration (use seek?)
type JobOperationInput struct {
	// Possible values are "audio", "subtitle" and "video"
	MediaType *JobMediaType `json:"media_type,omitempty"`
	Name      string        `json:"name"`
	PID       *int          `json:"pid,omitempty"`
}

// JobOperationOutput represents a job operation output
type JobOperationOutput struct {
	Name string `json:"name"`
	PID  *int   `json:"pid,omitempty"`
}
