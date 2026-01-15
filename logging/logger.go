package logging

import (
	"io"
	"os"
	"sync"
	"time"
)

type Logger interface {
	Debug(component, action, msg string)
	Info(component, action, msg string)
	Warn(component, action, msg string)
	Error(component, action, msg string)
	WithFields(fields Fields) Logger
	WithError(err error) Logger
	WithTraceID(traceID string) Logger
}

type StandardLogger struct {
	mu        sync.Mutex
	out       io.Writer
	formatter Formatter
	level     LogLevel
	fields    Fields
	traceID   string
	sanitize  bool
	errType   string
}

type LoggerConfig struct {
	Output    io.Writer
	Formatter Formatter
	Level     LogLevel
	Sanitize  bool
}

func NewLogger(cfg LoggerConfig) *StandardLogger {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	formatter := cfg.Formatter
	if formatter == nil {
		formatter = NewHumanFormatter(out)
	}

	return &StandardLogger{
		out:       out,
		formatter: formatter,
		level:     cfg.Level,
		fields:    make(Fields),
		sanitize:  cfg.Sanitize,
	}
}

func (l *StandardLogger) log(level LogLevel, component, action, msg string) {
	if !level.ShouldLog(l.level) {
		return
	}

	fields := l.fields
	if l.sanitize {
		fields = fields.Sanitize()
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Component: component,
		Action:    action,
		Message:   msg,
		Fields:    fields,
		TraceID:   l.traceID,
		ErrorType: l.errType,
	}

	if errField, ok := l.fields["error"]; ok {
		if errStr, ok := errField.(string); ok {
			entry.Error = errStr
		}
	}

	data, err := l.formatter.Format(entry)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.out.Write(data)
}

func (l *StandardLogger) Debug(component, action, msg string) {
	l.log(DEBUG, component, action, msg)
}

func (l *StandardLogger) Info(component, action, msg string) {
	l.log(INFO, component, action, msg)
}

func (l *StandardLogger) Warn(component, action, msg string) {
	l.log(WARN, component, action, msg)
}

func (l *StandardLogger) Error(component, action, msg string) {
	l.log(ERROR, component, action, msg)
}

func (l *StandardLogger) WithFields(fields Fields) Logger {
	newFields := make(Fields, len(l.fields)+len(fields))
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &StandardLogger{
		out:       l.out,
		formatter: l.formatter,
		level:     l.level,
		fields:    newFields,
		traceID:   l.traceID,
		sanitize:  l.sanitize,
		errType:   l.errType,
	}
}

func (l *StandardLogger) WithError(err error) Logger {
	if err == nil {
		return l
	}
	newLogger := l.WithFields(Fields{"error": err.Error()}).(*StandardLogger)
	newLogger.errType = ErrorType(err)
	return newLogger
}

func (l *StandardLogger) WithTraceID(traceID string) Logger {
	return &StandardLogger{
		out:       l.out,
		formatter: l.formatter,
		level:     l.level,
		fields:    l.fields,
		traceID:   traceID,
		sanitize:  l.sanitize,
		errType:   l.errType,
	}
}

type NopLogger struct{}

func (NopLogger) Debug(component, action, msg string)    {}
func (NopLogger) Info(component, action, msg string)     {}
func (NopLogger) Warn(component, action, msg string)     {}
func (NopLogger) Error(component, action, msg string)    {}
func (n NopLogger) WithFields(fields Fields) Logger      { return n }
func (n NopLogger) WithError(err error) Logger           { return n }
func (n NopLogger) WithTraceID(traceID string) Logger    { return n }
