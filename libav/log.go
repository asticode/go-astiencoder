package astilibav

import (
	"regexp"
	"strings"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

type EventLog struct {
	Fmt    string
	Level  astiav.LogLevel
	Msg    string
	Parent string
}

// TODO Process parent and update event's target
func HandleLogs(eh *astiencoder.EventHandler) {
	astiav.SetLogCallback(func(level astiav.LogLevel, fmt, msg, parent string) {
		// Emit event
		eh.Emit(astiencoder.Event{
			Name: EventNameLog,
			Payload: EventLog{
				Fmt:    fmt,
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
			case astiav.LogLevelDebug, astiav.LogLevelVerbose:
				l.Debug(msg)
			case astiav.LogLevelInfo:
				l.Info(msg)
			case astiav.LogLevelError, astiav.LogLevelFatal, astiav.LogLevelPanic:
				if v.Level == astiav.LogLevelFatal {
					msg = "FATAL! " + msg
				} else if v.Level == astiav.LogLevelPanic {
					msg = "PANIC! " + msg
				}
				l.Error(msg)
			case astiav.LogLevelWarning:
				l.Warn(msg)
			}
		}
		return false
	}
}
