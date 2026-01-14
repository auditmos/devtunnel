package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestNewApp(t *testing.T) {
	app := NewApp()
	assert.Equal(t, "devtunnel", app.Name)
	assert.Equal(t, "expose localhost to the internet", app.Usage)
	assert.Len(t, app.Commands, 3)
}

func TestServerCommand(t *testing.T) {
	app := NewApp()

	var found bool
	for _, cmd := range app.Commands {
		if cmd.Name == "server" {
			found = true
			assert.Equal(t, "start public gateway server", cmd.Usage)
			assert.GreaterOrEqual(t, len(cmd.Flags), 1)
		}
	}
	assert.True(t, found, "server command not found")
}

func TestClientCommand(t *testing.T) {
	app := NewApp()

	var found bool
	for _, cmd := range app.Commands {
		if cmd.Name == "start" {
			found = true
			assert.Equal(t, "expose local port to the internet", cmd.Usage)
			assert.Equal(t, "[port]", cmd.ArgsUsage)
		}
	}
	assert.True(t, found, "start command not found")
}

func TestHelpOutput(t *testing.T) {
	app := NewApp()
	err := app.Run([]string{"devtunnel", "--help"})
	require.NoError(t, err)
}

func TestVersionOutput(t *testing.T) {
	app := NewApp()
	err := app.Run([]string{"devtunnel", "--version"})
	require.NoError(t, err)
}

func TestServerCommandParsing(t *testing.T) {
	cmd := serverCommand()
	assert.Equal(t, "server", cmd.Name)
	assert.NotNil(t, cmd.Action)
}

func TestClientPortOptional(t *testing.T) {
	cmd := clientCommand()
	assert.Equal(t, "[port]", cmd.ArgsUsage)
	for _, f := range cmd.Flags {
		if sf, ok := f.(*cli.StringFlag); ok && sf.Name == "port" {
			assert.Equal(t, "3000", sf.Value)
		}
	}
}

func TestClientCommandParsing(t *testing.T) {
	cmd := clientCommand()
	assert.Equal(t, "start", cmd.Name)
	assert.NotNil(t, cmd.Action)
}

func TestClientServerFlagExists(t *testing.T) {
	cmd := clientCommand()
	var found bool
	for _, f := range cmd.Flags {
		if sf, ok := f.(*cli.StringFlag); ok && sf.Name == "server" {
			found = true
			assert.Equal(t, "localhost:8080", sf.Value)
		}
	}
	assert.True(t, found, "server flag not found")
}

func TestClientSafeFlagExists(t *testing.T) {
	cmd := clientCommand()
	var found bool
	for _, f := range cmd.Flags {
		if bf, ok := f.(*cli.BoolFlag); ok && bf.Name == "safe" {
			found = true
		}
	}
	assert.True(t, found, "safe flag not found")
}

func TestReplayCommand(t *testing.T) {
	app := NewApp()
	var found bool
	for _, cmd := range app.Commands {
		if cmd.Name == "replay" {
			found = true
			assert.Equal(t, "replay a shared request to localhost", cmd.Usage)
			assert.Equal(t, "<url>", cmd.ArgsUsage)
		}
	}
	assert.True(t, found, "replay command not found")
}

func TestReplayRequiresURL(t *testing.T) {
	app := NewApp()
	err := app.Run([]string{"devtunnel", "replay"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "url argument required")
}

func TestClientJSONFlagExists(t *testing.T) {
	cmd := clientCommand()
	var found bool
	for _, f := range cmd.Flags {
		if bf, ok := f.(*cli.BoolFlag); ok && bf.Name == "json" {
			found = true
		}
	}
	assert.True(t, found, "json flag not found")
}

func TestClientLogLevelFlagExists(t *testing.T) {
	cmd := clientCommand()
	var found bool
	for _, f := range cmd.Flags {
		if sf, ok := f.(*cli.StringFlag); ok && sf.Name == "log-level" {
			found = true
			assert.Equal(t, "info", sf.Value)
		}
	}
	assert.True(t, found, "log-level flag not found")
}

func TestClientLogFileFlagExists(t *testing.T) {
	cmd := clientCommand()
	var found bool
	for _, f := range cmd.Flags {
		if sf, ok := f.(*cli.StringFlag); ok && sf.Name == "log-file" {
			found = true
			assert.Empty(t, sf.Value)
		}
	}
	assert.True(t, found, "log-file flag not found")
}

func TestServerJSONFlagExists(t *testing.T) {
	cmd := serverCommand()
	var found bool
	for _, f := range cmd.Flags {
		if bf, ok := f.(*cli.BoolFlag); ok && bf.Name == "json" {
			found = true
		}
	}
	assert.True(t, found, "json flag not found on server")
}

func TestServerLogLevelFlagExists(t *testing.T) {
	cmd := serverCommand()
	var found bool
	for _, f := range cmd.Flags {
		if sf, ok := f.(*cli.StringFlag); ok && sf.Name == "log-level" {
			found = true
			assert.Equal(t, "info", sf.Value)
		}
	}
	assert.True(t, found, "log-level flag not found on server")
}

func TestServerLogFileFlagExists(t *testing.T) {
	cmd := serverCommand()
	var found bool
	for _, f := range cmd.Flags {
		if sf, ok := f.(*cli.StringFlag); ok && sf.Name == "log-file" {
			found = true
			assert.Empty(t, sf.Value)
		}
	}
	assert.True(t, found, "log-file flag not found on server")
}

func TestInitLoggerJSON(t *testing.T) {
	logger, cleanup, err := initLogger(true, "info", "", false)
	require.NoError(t, err)
	defer cleanup()
	assert.NotNil(t, logger)
}

func TestInitLoggerHuman(t *testing.T) {
	logger, cleanup, err := initLogger(false, "info", "", false)
	require.NoError(t, err)
	defer cleanup()
	assert.NotNil(t, logger)
}

func TestInitLoggerLevels(t *testing.T) {
	tests := []string{"debug", "info", "warn", "error"}
	for _, lvl := range tests {
		t.Run(lvl, func(t *testing.T) {
			logger, cleanup, err := initLogger(false, lvl, "", false)
			require.NoError(t, err)
			defer cleanup()
			assert.NotNil(t, logger)
		})
	}
}

func TestInitLoggerToFile(t *testing.T) {
	tmpFile := t.TempDir() + "/test.log"
	logger, cleanup, err := initLogger(false, "info", tmpFile, false)
	require.NoError(t, err)
	defer cleanup()
	assert.NotNil(t, logger)
}
