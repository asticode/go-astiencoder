package astiencoder

// Default event names
var (
	EventNameError             = "astiencoder.error"
	EventNameNodeClosed        = "astiencoder.node.closed"
	EventNameNodeContinued     = "astiencoder.node.continued"
	EventNameNodePaused        = "astiencoder.node.paused"
	EventNameNodeStarted       = "astiencoder.node.started"
	EventNameNodeStopped       = "astiencoder.node.stopped"
	EventNameStats             = "astiencoder.stats"
	EventNameWorkflowClosed    = "astiencoder.workflow.closed"
	EventNameWorkflowContinued = "astiencoder.workflow.continued"
	EventNameWorkflowPaused    = "astiencoder.workflow.paused"
	EventNameWorkflowStarted   = "astiencoder.workflow.started"
	EventNameWorkflowStopped   = "astiencoder.workflow.stopped"
	EventTypeClosed            = "closed"
	EventTypeContinued         = "continued"
	EventTypePaused            = "paused"
	EventTypeStarted           = "started"
	EventTypeStopped           = "stopped"
)

// Event is an event coming out of the encoder
type Event struct {
	Name    string
	Payload interface{}
	Target  interface{}
}

// EventError returns an error event
func EventError(target interface{}, err error) Event {
	return Event{
		Name:    EventNameError,
		Payload: err,
		Target:  target,
	}
}

// EventTypeTransformer represents a function capable of transforming an event type to an event name
type EventTypeTransformer func(eventType string) string

// EventTypeToNodeEventName is the node EventTypeTransformer
func EventTypeToNodeEventName(eventType string) string {
	switch eventType {
	case EventTypeClosed:
		return EventNameNodeClosed
	case EventTypeContinued:
		return EventNameNodeContinued
	case EventTypePaused:
		return EventNameNodePaused
	case EventTypeStarted:
		return EventNameNodeStarted
	case EventTypeStopped:
		return EventNameNodeStopped
	default:
		return ""
	}
}

// EventTypeToWorkflowEventName is the workflow EventTypeTransformer
func EventTypeToWorkflowEventName(eventType string) string {
	switch eventType {
	case EventTypeClosed:
		return EventNameWorkflowClosed
	case EventTypeContinued:
		return EventNameWorkflowContinued
	case EventTypePaused:
		return EventNameWorkflowPaused
	case EventTypeStarted:
		return EventNameWorkflowStarted
	case EventTypeStopped:
		return EventNameWorkflowStopped
	default:
		return ""
	}
}
