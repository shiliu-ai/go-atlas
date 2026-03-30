package errors

import (
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	e := New(CodeBadRequest, "bad request")
	if e.Code() != CodeBadRequest {
		t.Errorf("Code() = %d, want %d", e.Code(), CodeBadRequest)
	}
	if e.Message() != "bad request" {
		t.Errorf("Message() = %q, want %q", e.Message(), "bad request")
	}
	if e.MsgKey() != "" {
		t.Errorf("MsgKey() = %q, want empty", e.MsgKey())
	}
	if e.Unwrap() != nil {
		t.Error("Unwrap() should be nil")
	}
}

func TestNewT(t *testing.T) {
	e := NewT(CodeNotFound, "error.not_found", "user", 42)
	if e.Code() != CodeNotFound {
		t.Errorf("Code() = %d, want %d", e.Code(), CodeNotFound)
	}
	if e.Message() != "" {
		t.Errorf("Message() = %q, want empty", e.Message())
	}
	if e.MsgKey() != "error.not_found" {
		t.Errorf("MsgKey() = %q, want %q", e.MsgKey(), "error.not_found")
	}
	args := e.MsgArgs()
	if len(args) != 2 || args[0] != "user" || args[1] != 42 {
		t.Errorf("MsgArgs() = %v, want [user 42]", args)
	}
}

func TestWrap(t *testing.T) {
	cause := fmt.Errorf("connection refused")
	e := Wrap(CodeInternal, "db failed", cause)
	if e.Unwrap() != cause {
		t.Error("Unwrap() should return the cause")
	}
	if e.Code() != CodeInternal {
		t.Errorf("Code() = %d, want %d", e.Code(), CodeInternal)
	}
}

func TestWrapT(t *testing.T) {
	cause := fmt.Errorf("timeout")
	e := WrapT(CodeBadGateway, "error.upstream", cause, "svc-a")
	if e.Unwrap() != cause {
		t.Error("Unwrap() should return the cause")
	}
	if e.MsgKey() != "error.upstream" {
		t.Errorf("MsgKey() = %q, want %q", e.MsgKey(), "error.upstream")
	}
	if len(e.MsgArgs()) != 1 || e.MsgArgs()[0] != "svc-a" {
		t.Errorf("MsgArgs() = %v, want [svc-a]", e.MsgArgs())
	}
}

func TestError_ErrorString(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "plain message",
			err:  New(CodeBadRequest, "bad input"),
			want: "[400] bad input",
		},
		{
			name: "i18n key as fallback",
			err:  NewT(CodeNotFound, "error.not_found"),
			want: "[404] i18n:error.not_found",
		},
		{
			name: "with cause and plain message",
			err:  Wrap(CodeInternal, "db failed", fmt.Errorf("conn refused")),
			want: "[500] db failed: conn refused",
		},
		{
			name: "with cause and i18n key",
			err:  WrapT(CodeBadGateway, "error.upstream", fmt.Errorf("timeout")),
			want: "[502] i18n:error.upstream: timeout",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWithHTTPStatus_ReturnsClone(t *testing.T) {
	original := New(CodeBadRequest, "bad")
	modified := original.WithHTTPStatus(422)

	if modified.HTTPStatus() != 422 {
		t.Errorf("modified.HTTPStatus() = %d, want 422", modified.HTTPStatus())
	}
	if original.HTTPStatus() != 0 {
		t.Errorf("original.HTTPStatus() = %d, want 0 (should not be mutated)", original.HTTPStatus())
	}
	if original == modified {
		t.Error("WithHTTPStatus should return a new pointer, not the same object")
	}
}

func TestIs(t *testing.T) {
	t.Run("same code matches", func(t *testing.T) {
		e := New(CodeNotFound, "user not found")
		if !Is(e, ErrNotFound) {
			t.Error("errors.Is should match by code")
		}
	})

	t.Run("different code does not match", func(t *testing.T) {
		e := New(CodeNotFound, "not found")
		if Is(e, ErrBadRequest) {
			t.Error("errors.Is should not match different codes")
		}
	})

	t.Run("wrapped error matches by code", func(t *testing.T) {
		e := New(CodeNotFound, "user not found")
		wrapped := fmt.Errorf("outer: %w", e)
		if !Is(wrapped, ErrNotFound) {
			t.Error("errors.Is should match wrapped error by code")
		}
	})

	t.Run("non-Error target", func(t *testing.T) {
		e := New(CodeNotFound, "not found")
		if Is(e, fmt.Errorf("plain")) {
			t.Error("errors.Is should return false for non-*Error target")
		}
	})
}

func TestWithMessage(t *testing.T) {
	original := ErrNotFound
	modified := original.WithMessage("user not found")

	if modified.Message() != "user not found" {
		t.Errorf("Message() = %q, want %q", modified.Message(), "user not found")
	}
	if modified.MsgKey() != "" {
		t.Errorf("MsgKey() should be empty after WithMessage, got %q", modified.MsgKey())
	}
	if modified.Code() != CodeNotFound {
		t.Errorf("Code() = %d, want %d", modified.Code(), CodeNotFound)
	}
	if original.Message() != "not found" {
		t.Error("original should not be mutated")
	}
	if !Is(modified, ErrNotFound) {
		t.Error("modified error should still match ErrNotFound by code")
	}
}

func TestWithMsgKey(t *testing.T) {
	original := New(CodeBadRequest, "bad request")
	modified := original.WithMsgKey("error.validation", "field1")

	if modified.MsgKey() != "error.validation" {
		t.Errorf("MsgKey() = %q, want %q", modified.MsgKey(), "error.validation")
	}
	if modified.Message() != "" {
		t.Errorf("Message() should be empty after WithMsgKey, got %q", modified.Message())
	}
	if len(modified.MsgArgs()) != 1 || modified.MsgArgs()[0] != "field1" {
		t.Errorf("MsgArgs() = %v, want [field1]", modified.MsgArgs())
	}
	if original.Message() != "bad request" {
		t.Error("original should not be mutated")
	}
}

func TestFromError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		if FromError(nil) != nil {
			t.Error("FromError(nil) should return nil")
		}
	})

	t.Run("direct *Error", func(t *testing.T) {
		e := New(CodeNotFound, "not found")
		got := FromError(e)
		if got != e {
			t.Error("FromError should return the same *Error")
		}
	})

	t.Run("wrapped *Error", func(t *testing.T) {
		e := New(CodeNotFound, "not found")
		wrapped := fmt.Errorf("outer: %w", e)
		got := FromError(wrapped)
		if got != e {
			t.Error("FromError should extract *Error from wrapped error")
		}
	})

	t.Run("non-*Error", func(t *testing.T) {
		plain := fmt.Errorf("plain error")
		if FromError(plain) != nil {
			t.Error("FromError should return nil for non-*Error")
		}
	})
}
