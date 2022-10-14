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

type EventHandlerLogAdapterOptions struct {
	LogLevel        astiav.LogLevel
	LoggerLevelFunc func(l astiav.LogLevel) (ll astikit.LoggerLevel, processed, stop bool)
}

func EventHandlerLogAdapter(o EventHandlerLogAdapterOptions) astiencoder.EventHandlerLogAdapter {
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

		// Get logger level func
		llf := o.LoggerLevelFunc
		if llf == nil {
			llf = func(l astiav.LogLevel) (ll astikit.LoggerLevel, processed bool, stop bool) { return }
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
				ll, processed, stop := llf(v.Level)
				if stop {
					return false
				}
				if !processed {
					switch v.Level {
					case astiav.LogLevelDebug, astiav.LogLevelVerbose:
						ll = astikit.LoggerLevelDebug
					case astiav.LogLevelInfo:
						ll = astikit.LoggerLevelInfo
					case astiav.LogLevelError, astiav.LogLevelFatal, astiav.LogLevelPanic:
						if v.Level == astiav.LogLevelFatal {
							msg = "FATAL! " + msg
						} else if v.Level == astiav.LogLevelPanic {
							msg = "PANIC! " + msg
						}
						ll = astikit.LoggerLevelError
					case astiav.LogLevelWarning:
						ll = astikit.LoggerLevelWarn
					default:
						return false
					}
				}

				// Write
				l.Writek(ll, format, msg)
			}
			return false
		})
	}
}
