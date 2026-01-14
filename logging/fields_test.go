package logging

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithField(t *testing.T) {
	f := WithField("key", "value")
	assert.Equal(t, "value", f["key"])
}

func TestWithFields(t *testing.T) {
	original := Fields{"a": 1, "b": 2}
	copy := WithFields(original)

	assert.Equal(t, original, copy)

	copy["c"] = 3
	assert.NotContains(t, original, "c")
}

func TestWithError(t *testing.T) {
	t.Run("with error", func(t *testing.T) {
		err := errors.New("test error")
		f := WithError(err)
		assert.Equal(t, "test error", f["error"])
	})

	t.Run("nil error", func(t *testing.T) {
		f := WithError(nil)
		assert.Empty(t, f)
	})
}

func TestFields_Add(t *testing.T) {
	f := Fields{"a": 1}
	f.Add("b", 2)
	assert.Equal(t, 1, f["a"])
	assert.Equal(t, 2, f["b"])
}

func TestFields_Merge(t *testing.T) {
	f1 := Fields{"a": 1}
	f2 := Fields{"b": 2, "c": 3}
	f1.Merge(f2)

	assert.Equal(t, 1, f1["a"])
	assert.Equal(t, 2, f1["b"])
	assert.Equal(t, 3, f1["c"])
}

func TestFields_Sanitize(t *testing.T) {
	f := Fields{
		"username":      "john",
		"password":      "secret123",
		"token":         "abc123",
		"api_key":       "xyz789",
		"authorization": "Bearer token",
		"normal_field":  "visible",
	}

	sanitized := f.Sanitize()

	assert.Equal(t, "john", sanitized["username"])
	assert.Equal(t, "[REDACTED]", sanitized["password"])
	assert.Equal(t, "[REDACTED]", sanitized["token"])
	assert.Equal(t, "[REDACTED]", sanitized["api_key"])
	assert.Equal(t, "[REDACTED]", sanitized["authorization"])
	assert.Equal(t, "visible", sanitized["normal_field"])
}
