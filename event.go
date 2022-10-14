package astiencoder

type EventName string

type EventType string

// Default event names
var (
	EventNameError                EventName = "astiencoder.error"
	EventNameNodeChildAdded       EventName = "astiencoder.node.child.added"
	EventNameNodeChildRemoved     EventName = "astiencoder.node.child.removed"
	EventNameNodeClosed           EventName = "astiencoder.node.closed"
	EventNameNodeContinued        EventName = "astiencoder.node.continued"
	EventNameNodePaused           EventName = "astiencoder.node.paused"
	EventNameNodeStarted          EventName = "astiencoder.node.started"
	EventNameNodeStopped          EventName = "astiencoder.node.stopped"
	EventNameStats                EventName = "astiencoder.stats"
	EventNameWorkflowChildAdded   EventName = "astiencoder.workflow.child.added"
	EventNameWorkflowChildRemoved EventName = "astiencoder.workflow.child.removed"
	EventNameWorkflowClosed       EventName = "astiencoder.workflow.closed"
	EventNameWorkflowContinued    EventName = "astiencoder.workflow.continued"
	EventNameWorkflowPaused       EventName = "astiencoder.workflow.paused"
	EventNameWorkflowStarted      EventName = "astiencoder.workflow.started"
	EventNameWorkflowStopped      EventName = "astiencoder.workflow.stopped"
	EventTypeChildAdded           EventType = "child.added"
	EventTypeChildRemoved         EventType = "child.removed"
	EventTypeClosed               EventType = "closed"
	EventTypeContinued            EventType = "continued"
	EventTypePaused               EventType = "paused"
	EventTypeStarted              EventType = "started"
	EventTypeStopped              EventType = "stopped"
)

// Event is an event coming out of the encoder
type Event struct {
	Name    EventName
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
type EventTypeTransformer func(eventType EventType) EventName

// EventTypeToNodeEventName is the node EventTypeTransformer
func EventTypeToNodeEventName(eventType EventType) EventName {
	switch eventType {
	case EventTypeChildAdded:
		return EventNameNodeChildAdded
	case EventTypeChildRemoved:
		return EventNameNodeChildRemoved
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
func EventTypeToWorkflowEventName(eventType EventType) EventName {
	switch eventType {
	case EventTypeChildAdded:
		return EventNameWorkflowChildAdded
	case EventTypeChildRemoved:
		return EventNameWorkflowChildRemoved
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
