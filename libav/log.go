package astilibav

import (
	"regexp"
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
	IgnoredLogMessages []*regexp.Regexp
}

func LoggerEventHandlerAdapter(o LoggerEventHandlerAdapterOptions, i astikit.StdLogger, h *astiencoder.EventHandler) {
	h.AddForEventName(EventNameLog, loggerEventHandlerCallback(o, astikit.AdaptStdLogger(i)))
}

func loggerEventHandlerCallback(o LoggerEventHandlerAdapterOptions, l astikit.CompleteLogger) astiencoder.EventCallback {
	return func(e astiencoder.Event) bool {
		if v, ok := e.Payload.(EventLog); ok {
			// Sanitize
			msg := strings.TrimSpace(v.Msg)
			if msg == "" {
				return false
			}

			// Check ignored messages
			for _, r := range o.IgnoredLogMessages {
				if len(r.FindIndex([]byte(msg))) > 0 {
					return false
				}
			}

			// Add prefix
			msg = "astilibav: " + msg

			// Add parent
			if strings.Index(v.Parent, "0x") == 0 {
				msg += " (" + v.Parent + ")"
			}

			// Add level
			switch v.Level {
			case avutil.AV_LOG_DEBUG, avutil.AV_LOG_VERBOSE:
				l.Debug(msg)
			case avutil.AV_LOG_INFO:
				l.Info(msg)
			case avutil.AV_LOG_ERROR, avutil.AV_LOG_FATAL, avutil.AV_LOG_PANIC:
				if v.Level == avutil.AV_LOG_FATAL {
					msg = "FATAL! " + msg
				} else if v.Level == avutil.AV_LOG_PANIC {
					msg = "PANIC! " + msg
				}
				l.Error(msg)
			case avutil.AV_LOG_WARNING:
				l.Warn(msg)
			}
		}
		return false
	}
}
