package logging

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{DEBUG, "debug"},
		{INFO, "info"},
		{WARN, "warn"},
		{ERROR, "error"},
		{LogLevel(99), "unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.level.String())
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  LogLevel
	}{
		{"debug", DEBUG},
		{"DEBUG", DEBUG},
		{"info", INFO},
		{"INFO", INFO},
		{"warn", WARN},
		{"warning", WARN},
		{"WARN", WARN},
		{"error", ERROR},
		{"ERROR", ERROR},
		{"invalid", INFO},
		{"", INFO},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, ParseLevel(tt.input))
	}
}

func TestLogLevel_ShouldLog(t *testing.T) {
	tests := []struct {
		level LogLevel
		min   LogLevel
		want  bool
	}{
		{DEBUG, DEBUG, true},
		{DEBUG, INFO, false},
		{INFO, DEBUG, true},
		{INFO, INFO, true},
		{INFO, WARN, false},
		{WARN, INFO, true},
		{ERROR, WARN, true},
		{ERROR, ERROR, true},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.level.ShouldLog(tt.min))
	}
}
