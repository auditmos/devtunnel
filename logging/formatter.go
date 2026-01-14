package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mattn/go-isatty"
)

type Formatter interface {
	Format(entry LogEntry) ([]byte, error)
}

type JSONFormatter struct{}

func (f *JSONFormatter) Format(entry LogEntry) ([]byte, error) {
	output := map[string]interface{}{
		"timestamp": entry.Timestamp.Format(time.RFC3339),
		"level":     entry.Level.String(),
		"component": entry.Component,
		"action":    entry.Action,
		"message":   entry.Message,
	}

	if len(entry.Fields) > 0 {
		output["fields"] = entry.Fields
	}

	if entry.Error != "" {
		output["error"] = entry.Error
	}

	if entry.TraceID != "" {
		output["trace_id"] = entry.TraceID
	}

	data, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	return append(data, '\n'), nil
}

type HumanFormatter struct {
	colorEnabled bool
}

func NewHumanFormatter(w io.Writer) *HumanFormatter {
	colorEnabled := false
	if f, ok := w.(*os.File); ok {
		colorEnabled = isatty.IsTerminal(f.Fd())
	}
	return &HumanFormatter{colorEnabled: colorEnabled}
}

func (f *HumanFormatter) Format(entry LogEntry) ([]byte, error) {
	ts := entry.Timestamp.Format("15:04:05")
	level := f.colorLevel(entry.Level)

	msg := fmt.Sprintf("%s %s [%s] %s: %s",
		ts, level, entry.Component, entry.Action, entry.Message)

	if len(entry.Fields) > 0 {
		msg += " " + formatFields(entry.Fields)
	}

	if entry.Error != "" {
		msg += fmt.Sprintf(" error=%s", entry.Error)
	}

	if entry.TraceID != "" {
		msg += fmt.Sprintf(" trace_id=%s", entry.TraceID)
	}

	return []byte(msg + "\n"), nil
}

func (f *HumanFormatter) colorLevel(l LogLevel) string {
	name := l.String()
	if !f.colorEnabled {
		return fmt.Sprintf("%-5s", name)
	}

	var color string
	switch l {
	case DEBUG:
		color = "\033[36m" // cyan
	case INFO:
		color = "\033[32m" // green
	case WARN:
		color = "\033[33m" // yellow
	case ERROR:
		color = "\033[31m" // red
	default:
		color = ""
	}
	return fmt.Sprintf("%s%-5s\033[0m", color, name)
}

func formatFields(f Fields) string {
	result := "["
	first := true
	for k, v := range f {
		if !first {
			result += ", "
		}
		result += fmt.Sprintf("%s=%v", k, v)
		first = false
	}
	return result + "]"
}
