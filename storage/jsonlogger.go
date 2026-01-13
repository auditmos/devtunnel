package storage

import (
	"encoding/json"
	"io"
	"time"

	"github.com/auditmos/devtunnel/tunnel"
)

type JSONLogEntry struct {
	Timestamp       int64             `json:"timestamp"`
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	RequestHeaders  map[string]string `json:"request_headers"`
	RequestBody     string            `json:"request_body"`
	StatusCode      int               `json:"status_code"`
	ResponseHeaders map[string]string `json:"response_headers"`
	ResponseBody    string            `json:"response_body"`
	DurationMs      int64             `json:"duration_ms"`
}

type JSONLogger struct {
	w        io.Writer
	scrubber *Scrubber
}

func NewJSONLogger(w io.Writer, scrubber *Scrubber) *JSONLogger {
	return &JSONLogger{w: w, scrubber: scrubber}
}

func (l *JSONLogger) Log(input *tunnel.RequestLog) error {
	reqHeaders := input.RequestHeaders
	respHeaders := input.ResponseHeaders
	if l.scrubber != nil {
		reqHeaders = l.scrubber.ScrubHeaders(reqHeaders)
		respHeaders = l.scrubber.ScrubHeaders(respHeaders)
	}

	entry := JSONLogEntry{
		Timestamp:       time.Now().UnixMilli(),
		Method:          input.Method,
		URL:             input.URL,
		RequestHeaders:  reqHeaders,
		RequestBody:     string(input.RequestBody),
		StatusCode:      input.StatusCode,
		ResponseHeaders: respHeaders,
		ResponseBody:    string(input.ResponseBody),
		DurationMs:      input.DurationMs,
	}

	enc := json.NewEncoder(l.w)
	return enc.Encode(entry)
}

type MultiLogger struct {
	loggers []tunnel.RequestLogger
}

func NewMultiLogger(loggers ...tunnel.RequestLogger) *MultiLogger {
	return &MultiLogger{loggers: loggers}
}

func (m *MultiLogger) Log(input *tunnel.RequestLog) error {
	for _, l := range m.loggers {
		if err := l.Log(input); err != nil {
			return err
		}
	}
	return nil
}
