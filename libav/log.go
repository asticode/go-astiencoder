package astilibav

import (
	"strings"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avutil"
)

type EventLog struct {
	Level  int
	Msg    string
	Parent string
}

// TODO Process parent and update event's target
func HandleLogs(eh *astiencoder.EventHandler) {
	avutil.AvLogSetCallback(func(level int, msg, parent string) {
		// Emit event
		eh.Emit(astiencoder.Event{
			Name: EventNameLog,
			Payload: EventLog{
				Level:  level,
				Msg:    msg,
				Parent: parent,
			},
		})
	})
}

type LoggerEventHandlerAdapterOptions struct {
	IgnoredLogMessages []string
}

func LoggerEventHandlerAdapter(o LoggerEventHandlerAdapterOptions, i astikit.StdLogger, h *astiencoder.EventHandler) {
	// Create logger
	l := astikit.AdaptStdLogger(i)

	// Index ignored log messages
	is := make(map[string]bool)
	for _, msg := range o.IgnoredLogMessages {
		is[msg] = true
	}

	// Log
	h.AddForEventName(EventNameLog, func(e astiencoder.Event) bool {
		if v, ok := e.Payload.(EventLog); ok {
			msg := strings.TrimSpace(v.Msg)
			if msg == "" {
				return false
			}
			if _, ok := is[msg]; ok {
				return false
			}
			msg = "astilibav: " + msg
			if strings.Index(v.Parent, "0x") == 0 {
				msg += " (" + v.Parent + ")"
			}
			switch v.Level {
			case avutil.AV_LOG_DEBUG, avutil.AV_LOG_VERBOSE:
				l.Debug(msg)
			case avutil.AV_LOG_INFO:
				l.Info(msg)
			case avutil.AV_LOG_ERROR:
				l.Error(msg)
			case avutil.AV_LOG_WARNING:
				l.Warn(msg)
			case avutil.AV_LOG_FATAL, avutil.AV_LOG_PANIC:
				l.Fatal(msg)
			}
		}
		return false
	})
}
