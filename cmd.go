package astiencoder

// Cmd represents a command
type Cmd struct {
	Name    string      `json:"name"`
	Payload interface{} `json:"payload"`
}

// CmdHandler represents an object capable of handling a cmd
type CmdHandler interface {
	HandleCmd(c Cmd)
}
