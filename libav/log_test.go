package astilibav

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

type mockedStdLogger struct{ ss []string }

func newMockedStdLogger() *mockedStdLogger {
	return &mockedStdLogger{
		ss: []string{},
	}
}

func (l *mockedStdLogger) Fatal(v ...interface{}) { l.Print(v...) }

func (l *mockedStdLogger) Fatalf(format string, v ...interface{}) { l.Printf(format, v...) }

func (l *mockedStdLogger) Print(v ...interface{}) { l.ss = append(l.ss, fmt.Sprint(v...)) }

func (l *mockedStdLogger) Printf(format string, v ...interface{}) {
	l.ss = append(l.ss, fmt.Sprintf(format, v...))
}

func TestLog(t *testing.T) {
	l := newMockedStdLogger()
	c := loggerEventHandlerCallback(LoggerEventHandlerAdapterOptions{
		IgnoredLogMessages: []*regexp.Regexp{
			regexp.MustCompile("^test2$"),
			regexp.MustCompile(`[\w]+_pattern`),
		},
	}, astikit.AdaptStdLogger(l))
	c(astiencoder.Event{Payload: EventLog{Level: astiav.LogLevelInfo, Msg: "test1"}})
	c(astiencoder.Event{Payload: EventLog{Level: astiav.LogLevelInfo, Msg: "test2"}})
	c(astiencoder.Event{Payload: EventLog{Level: astiav.LogLevelInfo, Msg: "test3"}})
	c(astiencoder.Event{Payload: EventLog{Level: astiav.LogLevelInfo, Msg: "test_pattern"}})
	if e, g := []string{"astilibav: test1", "astilibav: test3"}, l.ss; !reflect.DeepEqual(e, g) {
		t.Errorf("expected %+v, got %+v", e, g)
	}
}
