package astiencoder

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/asticode/go-astikit"
)

type logLevel string

const (
	logLevelDebug logLevel = "debug"
	logLevelError logLevel = "error"
	logLevelInfo  logLevel = "info"
	logLevelWarn  logLevel = "warn"
)

type EventLogger struct {
	cancel               context.CancelFunc
	ctx                  context.Context
	is                   map[string]*eventLoggerItem // Indexed by key
	l                    astikit.CompleteLogger
	m                    *sync.Mutex // Locks p
	messageMergingPeriod time.Duration
}

type eventLoggerItem struct {
	count     int
	createdAt time.Time
	key       string
	l         logLevel
	msg       string
}

func newEventLoggerItem(key, msg string, l logLevel) *eventLoggerItem {
	return &eventLoggerItem{
		createdAt: time.Now(),
		key:       key,
		l:         l,
		msg:       msg,
	}
}

func WithMessageMerging(period time.Duration) EventHandlerLogOption {
	return func(_ *EventHandler, l *EventLogger) {
		l.messageMergingPeriod = period
	}
}

func newEventLogger(i astikit.StdLogger) *EventLogger {
	return &EventLogger{
		is: make(map[string]*eventLoggerItem),
		l:  astikit.AdaptStdLogger(i),
		m:  &sync.Mutex{},
	}
}

func (l *EventLogger) Start(ctx context.Context) *EventLogger {
	// Create context
	l.ctx, l.cancel = context.WithCancel(ctx)

	// No need to start anything
	if l.messageMergingPeriod == 0 {
		return l
	}

	// Execute in a goroutine since this is blocking
	go func() {
		// Create ticker
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()

		// Loop
		for {
			select {
			case <-t.C:
				l.tick()
			case <-l.ctx.Done():
				return
			}
		}
	}()
	return l
}

func (l *EventLogger) Close() {
	if l.cancel != nil {
		l.cancel()
	}
	l.purge()
}

func (l *EventLogger) tick() {
	// Lock
	l.m.Lock()
	defer l.m.Unlock()

	// Get now
	n := time.Now()

	// Loop through items
	for k, i := range l.is {
		// Period has been reached
		if n.Sub(i.createdAt) > l.messageMergingPeriod {
			l.dumpItem(k, i)
		}
	}
}

func (l *EventLogger) purge() {
	// Lock
	l.m.Lock()
	defer l.m.Unlock()

	// Loop through items
	for k, i := range l.is {
		l.dumpItem(k, i)
	}
}

func (l *EventLogger) dumpItem(k string, i *eventLoggerItem) {
	if i.count > 1 {
		l.write(fmt.Sprintf("astiencoder: pattern repeated %d times: %s", i.count, i.key), i.l)
	} else if i.count == 1 {
		l.write("astiencoder: pattern repeated once: "+i.msg, i.l)
	}
	delete(l.is, k)
}

func (l *EventLogger) process(key, msg string, lv logLevel) {
	// Merge messages
	if l.messageMergingPeriod > 0 {
		// Merge
		if stop := l.merge(key, msg, lv); stop {
			return
		}
	}

	// Write
	l.write(msg, lv)
}

func (l *EventLogger) merge(key, msg string, lv logLevel) (stop bool) {
	// Lock
	l.m.Lock()
	defer l.m.Unlock()

	// Create final key
	k := string(lv) + ":" + key

	// Check whether item exists
	i, ok := l.is[k]
	if ok {
		i.count++
		return true
	}

	// Create item
	l.is[k] = newEventLoggerItem(key, msg, lv)
	return false
}

func (l *EventLogger) write(msg string, lv logLevel) {
	switch lv {
	case logLevelDebug:
		l.l.Debug(msg)
	case logLevelError:
		l.l.Error(msg)
	case logLevelWarn:
		l.l.Warn(msg)
	default:
		l.l.Info(msg)
	}
}

func (l *EventLogger) Debugk(key, msg string) {
	l.process(key, msg, logLevelDebug)
}

func (l *EventLogger) Errorf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.process(msg, msg, logLevelError)
}

func (l *EventLogger) Errork(key, msg string) {
	l.process(key, msg, logLevelError)
}

func (l *EventLogger) Infof(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.process(msg, msg, logLevelInfo)
}

func (l *EventLogger) Infok(key, msg string) {
	l.process(key, msg, logLevelInfo)
}

func (l *EventLogger) Warnk(key, msg string) {
	l.process(key, msg, logLevelWarn)

}
