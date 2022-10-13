package astilibav

import (
	"strings"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

type EventLog struct {
	Format string
	Level  astiav.LogLevel
	Msg    string
	Parent string
}

type WithLogOptions struct {
	LogLevel        astiav.LogLevel
	LoggerLevelFunc func(l astiav.LogLevel, msg *string) (ll astikit.LoggerLevel, ok bool)
}

func defaultLoggerLevelsFunc(l astiav.LogLevel, msg *string) (ll astikit.LoggerLevel, ok bool) {
	switch l {
	case astiav.LogLevelDebug, astiav.LogLevelVerbose:
		ll = astikit.LoggerLevelDebug
	case astiav.LogLevelInfo:
		ll = astikit.LoggerLevelInfo
	case astiav.LogLevelError, astiav.LogLevelFatal, astiav.LogLevelPanic:
		if l == astiav.LogLevelFatal {
			*msg = "FATAL! " + *msg
		} else if l == astiav.LogLevelPanic {
			*msg = "PANIC! " + *msg
		}
		ll = astikit.LoggerLevelError
	case astiav.LogLevelWarning:
		ll = astikit.LoggerLevelWarn
	default:
		return
	}
	ok = true
	return
}

func WithLog(o WithLogOptions) astiencoder.EventHandlerLogOption {
	return func(h *astiencoder.EventHandler, l *astiencoder.EventLogger) {
		// Set log level
		astiav.SetLogLevel(o.LogLevel)

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

		// Get logger levels func
		llf := o.LoggerLevelFunc
		if llf == nil {
			llf = defaultLoggerLevelsFunc
		}

		// Handle log
		h.AddForEventName(EventNameLog, func(e astiencoder.Event) bool {
			if v, ok := e.Payload.(EventLog); ok {
				// Sanitize
				msg := strings.TrimSpace(v.Msg)
				if msg == "" {
					return false
				}
				format := strings.TrimSpace(v.Format)
				if format == "%s" {
					format = msg
				}

				// Add prefix
				format = "astilibav: " + format
				msg = "astilibav: " + msg

				// Add parent
				if strings.Index(v.Parent, "0x") == 0 {
					msg += " (" + v.Parent + ")"
				}

				// Get level
				ll, ok := llf(v.Level, &msg)
				if !ok {
					return false
				}

				// Write
				l.Writek(ll, format, msg)
			}
			return false
		})
	}
}
