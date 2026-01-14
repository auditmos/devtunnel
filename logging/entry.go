package logging

import "time"

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     LogLevel  `json:"level"`
	Component string    `json:"component"`
	Action    string    `json:"action"`
	Message   string    `json:"message"`
	Fields    Fields    `json:"fields,omitempty"`
	Error     string    `json:"error,omitempty"`
	TraceID   string    `json:"trace_id,omitempty"`
}
