package astilibav

import (
	"strings"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
)

type EventLog struct {
	Format string
	Level  astiav.LogLevel
	Msg    string
	Parent string
}

func WithLog(lvl astiav.LogLevel) astiencoder.EventHandlerLogOption {
	return func(h *astiencoder.EventHandler, l *astiencoder.EventLogger) {
		// Set log level
		astiav.SetLogLevel(lvl)

		// Set log callback
		// TODO Process parent and update event's target
		astiav.SetLogCallback(func(level astiav.LogLevel, fmt, msg, parent string) {
			// Emit event
			h.Emit(astiencoder.Event{
				Name: EventNameLog,
				Payload: EventLog{
					Format: fmt,
					Level:  level,
					Msg:    msg,
					Parent: parent,
				},
			})
		})

		// Handle log
		h.AddForEventName(EventNameLog, func(e astiencoder.Event) bool {
			if v, ok := e.Payload.(EventLog); ok {
				// Sanitize
				format := strings.TrimSpace(v.Format)
				msg := strings.TrimSpace(v.Msg)
				if msg == "" {
					return false
				}

				// Add prefix
				format = "astilibav: " + format
				msg = "astilibav: " + msg

				// Add parent
				if strings.Index(v.Parent, "0x") == 0 {
					msg += " (" + v.Parent + ")"
				}

				// Add level
				switch v.Level {
				case astiav.LogLevelDebug, astiav.LogLevelVerbose:
					l.Debugk(format, msg)
				case astiav.LogLevelInfo:
					l.Infok(format, msg)
				case astiav.LogLevelError, astiav.LogLevelFatal, astiav.LogLevelPanic:
					if v.Level == astiav.LogLevelFatal {
						msg = "FATAL! " + msg
					} else if v.Level == astiav.LogLevelPanic {
						msg = "PANIC! " + msg
					}
					l.Errork(format, msg)
				case astiav.LogLevelWarning:
					l.Warnk(format, msg)
				}
			}
			return false
		})
	}
}
