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
			assert.Equal(t, "<port>", cmd.ArgsUsage)
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

func TestClientRequiresPort(t *testing.T) {
	app := NewApp()
	err := app.Run([]string{"devtunnel", "start"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "port argument required")
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
