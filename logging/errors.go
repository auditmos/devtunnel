package logging

import (
	"errors"
	"reflect"
)

type TypedError interface {
	error
	Type() string
}

type ContextError struct {
	Op      string
	Err     error
	ErrType string
}

func (e *ContextError) Error() string {
	if e.Op != "" && e.Err != nil {
		return e.Op + ": " + e.Err.Error()
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Op
}

func (e *ContextError) Type() string {
	if e.ErrType != "" {
		return e.ErrType
	}
	return inferErrorType(e.Err)
}

func (e *ContextError) Unwrap() error {
	return e.Err
}

func WrapError(op string, err error) *ContextError {
	if err == nil {
		return nil
	}
	return &ContextError{Op: op, Err: err}
}

func WrapErrorWithType(op string, err error, errType string) *ContextError {
	if err == nil {
		return nil
	}
	return &ContextError{Op: op, Err: err, ErrType: errType}
}

func inferErrorType(err error) string {
	if err == nil {
		return ""
	}

	var typed TypedError
	if errors.As(err, &typed) {
		return typed.Type()
	}

	t := reflect.TypeOf(err)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

func ErrorType(err error) string {
	if err == nil {
		return ""
	}

	var ctx *ContextError
	if errors.As(err, &ctx) {
		return ctx.Type()
	}

	var typed TypedError
	if errors.As(err, &typed) {
		return typed.Type()
	}

	return inferErrorType(err)
}
