package astiencoder

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/asticode/go-astikit"
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
	ll        astikit.LoggerLevel
	msg       string
}

func newEventLoggerItem(ll astikit.LoggerLevel, key, msg string) *eventLoggerItem {
	return &eventLoggerItem{
		createdAt: time.Now(),
		key:       key,
		ll:        ll,
		msg:       msg,
	}
}

func MessageMergingEventHandlerLogAdapter(period time.Duration) EventHandlerLogAdapter {
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
		l.l.Write(i.ll, fmt.Sprintf("astiencoder: pattern repeated %d times: %s", i.count, i.key))
	} else if i.count == 1 {
		l.l.Write(i.ll, "astiencoder: pattern repeated once: "+i.msg)
	}
	delete(l.is, k)
}

func (l *EventLogger) process(ll astikit.LoggerLevel, key, msg string) {
	// Merge messages
	if l.messageMergingPeriod > 0 {
		// Merge
		if stop := l.merge(ll, key, msg); stop {
			return
		}
	}

	// Write
	l.l.Write(ll, msg)
}

func (l *EventLogger) merge(ll astikit.LoggerLevel, key, msg string) (stop bool) {
	// Lock
	l.m.Lock()
	defer l.m.Unlock()

	// Create final key
	k := ll.String() + ":" + key

	// Check whether item exists
	i, ok := l.is[k]
	if ok {
		i.count++
		return true
	}

	// Create item
	l.is[k] = newEventLoggerItem(ll, key, msg)
	return false
}

func (l *EventLogger) Writek(ll astikit.LoggerLevel, key, msg string) {
	l.process(ll, key, msg)
}

func (l *EventLogger) Writef(ll astikit.LoggerLevel, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.process(ll, msg, msg)
}
