package storage

import (
	"time"

	"github.com/auditmos/devtunnel/tunnel"
	"github.com/oklog/ulid/v2"
)

type DBLogger struct {
	repo     *SQLiteRequestRepo
	tunnelID string
	scrubber *Scrubber
}

func NewDBLogger(repo *SQLiteRequestRepo, tunnelID string, safeMode bool) *DBLogger {
	var scrubber *Scrubber
	if safeMode {
		scrubber = NewScrubber()
	}
	return &DBLogger{repo: repo, tunnelID: tunnelID, scrubber: scrubber}
}

func (l *DBLogger) SetTunnelID(id string) {
	l.tunnelID = id
}

func (l *DBLogger) Log(input *tunnel.RequestLog) error {
	reqHeaders := input.RequestHeaders
	respHeaders := input.ResponseHeaders
	if l.scrubber != nil {
		reqHeaders = l.scrubber.ScrubHeaders(reqHeaders)
		respHeaders = l.scrubber.ScrubHeaders(respHeaders)
	}

	req := &Request{
		ID:              ulid.Make().String(),
		TunnelID:        l.tunnelID,
		Timestamp:       time.Now().UnixMilli(),
		Method:          input.Method,
		URL:             input.URL,
		RequestHeaders:  reqHeaders,
		RequestBody:     input.RequestBody,
		StatusCode:      input.StatusCode,
		ResponseHeaders: respHeaders,
		ResponseBody:    input.ResponseBody,
		DurationMs:      input.DurationMs,
		CreatedAt:       time.Now().UnixMilli(),
	}
	return l.repo.Save(req)
}
