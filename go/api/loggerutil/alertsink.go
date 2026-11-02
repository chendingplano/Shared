package loggerutil

import (
	"fmt"
	"sync/atomic"
	"time"
)

// AlertRecord is a WARN or ERROR event emitted by a JimoLogger.
type AlertRecord struct {
	Level    string // "WARN" or "ERROR"
	Message  string // message as passed by the caller, without the req prefix
	ReqID    string
	Loc      string // the loc the logger was created with
	CallFlow string
	Attrs    map[string]string // key-value args, values rendered with fmt.Sprint
	Time     time.Time
}

// AlertSink receives every WARN/ERROR. It is called synchronously on the
// logging goroutine, so it must be fast and must never log through a
// JimoLogger (that would recurse).
type AlertSink func(AlertRecord)

var alertSink atomic.Pointer[AlertSink]

// SetAlertSink installs the sink (nil removes it). The shared library cannot
// import sysdatastores from here (import cycle), so sysdatastores registers
// its database writer through this hook.
func SetAlertSink(sink AlertSink) {
	if sink == nil {
		alertSink.Store(nil)
		return
	}
	alertSink.Store(&sink)
}

func (l *JimoLoggerImpl) emitAlert(level, message, callFlow string, args []any) {
	sink := alertSink.Load()
	if sink == nil {
		return
	}
	attrs := make(map[string]string, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		key := fmt.Sprint(args[i])
		if i+1 < len(args) {
			attrs[key] = fmt.Sprint(args[i+1])
		} else {
			attrs[key] = ""
		}
	}
	(*sink)(AlertRecord{
		Level:    level,
		Message:  message,
		ReqID:    l.reqID,
		Loc:      l.loc,
		CallFlow: callFlow,
		Attrs:    attrs,
		Time:     time.Now(),
	})
}
