package response

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shiliu-ai/go-atlas/errors"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func performRequest(handler gin.HandlerFunc) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handler(c)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) R {
	t.Helper()
	var r R
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return r
}

func TestOK(t *testing.T) {
	w := performRequest(func(c *gin.Context) {
		OK(c, map[string]string{"key": "value"})
	})

	if w.Code != http.StatusOK {
		t.Errorf("HTTP status = %d, want %d", w.Code, http.StatusOK)
	}
	r := decodeBody(t, w)
	if r.Code != 0 {
		t.Errorf("Code = %d, want 0", r.Code)
	}
	if r.Message != "ok" {
		t.Errorf("Message = %q, want %q", r.Message, "ok")
	}
	if r.Data == nil {
		t.Error("Data should not be nil")
	}
}

func TestErr_WithPlainError(t *testing.T) {
	e := errors.New(errors.CodeBadRequest, "invalid input")
	w := performRequest(func(c *gin.Context) {
		Err(c, e)
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("HTTP status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	r := decodeBody(t, w)
	if r.Code != int(errors.CodeBadRequest) {
		t.Errorf("Code = %d, want %d", r.Code, errors.CodeBadRequest)
	}
	if r.Message != "invalid input" {
		t.Errorf("Message = %q, want %q", r.Message, "invalid input")
	}
}

func TestErr_WithI18nKey(t *testing.T) {
	e := errors.NewT(errors.CodeNotFound, "response.internal_error")
	w := performRequest(func(c *gin.Context) {
		Err(c, e)
	})

	if w.Code != http.StatusNotFound {
		t.Errorf("HTTP status = %d, want %d", w.Code, http.StatusNotFound)
	}
	r := decodeBody(t, w)
	// Without i18n bundle set, falls back to built-in English messages.
	if r.Message != "internal error" {
		t.Errorf("Message = %q, want %q", r.Message, "internal error")
	}
}

func TestErr_WithHTTPStatusOverride(t *testing.T) {
	e := errors.New(errors.CodeBadRequest, "validation failed").WithHTTPStatus(422)
	w := performRequest(func(c *gin.Context) {
		Err(c, e)
	})

	if w.Code != 422 {
		t.Errorf("HTTP status = %d, want 422", w.Code)
	}
	r := decodeBody(t, w)
	if r.Code != int(errors.CodeBadRequest) {
		t.Errorf("Code = %d, want %d", r.Code, errors.CodeBadRequest)
	}
}

func TestErr_UnhandledError(t *testing.T) {
	w := performRequest(func(c *gin.Context) {
		Err(c, fmt.Errorf("something unexpected"))
	})

	if w.Code != http.StatusInternalServerError {
		t.Errorf("HTTP status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	r := decodeBody(t, w)
	if r.Code != int(errors.CodeInternal) {
		t.Errorf("Code = %d, want %d", r.Code, errors.CodeInternal)
	}
}

func TestFail(t *testing.T) {
	w := performRequest(func(c *gin.Context) {
		Fail(c, errors.CodeForbidden, "access denied")
	})

	if w.Code != http.StatusForbidden {
		t.Errorf("HTTP status = %d, want %d", w.Code, http.StatusForbidden)
	}
	r := decodeBody(t, w)
	if r.Code != int(errors.CodeForbidden) {
		t.Errorf("Code = %d, want %d", r.Code, errors.CodeForbidden)
	}
	if r.Message != "access denied" {
		t.Errorf("Message = %q, want %q", r.Message, "access denied")
	}
}

func TestFailT(t *testing.T) {
	w := performRequest(func(c *gin.Context) {
		FailT(c, errors.CodeBadRequest, "response.ok")
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("HTTP status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	r := decodeBody(t, w)
	// Falls back to built-in English: "ok"
	if r.Message != "ok" {
		t.Errorf("Message = %q, want %q", r.Message, "ok")
	}
}

func TestAbortErr(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	AbortErr(c, errors.New(errors.CodeUnauthorized, "no token"))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("HTTP status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if !c.IsAborted() {
		t.Error("expected context to be aborted")
	}
}

func TestCodeToHTTPStatus(t *testing.T) {
	tests := []struct {
		code errors.Code
		want int
	}{
		{errors.CodeBadRequest, http.StatusBadRequest},
		{errors.CodeUnauthorized, http.StatusUnauthorized},
		{errors.CodeForbidden, http.StatusForbidden},
		{errors.CodeNotFound, http.StatusNotFound},
		{errors.CodeConflict, http.StatusConflict},
		{errors.CodeTooManyRequests, http.StatusTooManyRequests},
		{errors.CodeInternal, http.StatusInternalServerError},
		{errors.CodeBadGateway, http.StatusBadGateway},
		{errors.CodeServiceUnavail, http.StatusServiceUnavailable},
		{errors.Code(405), http.StatusMethodNotAllowed},
		{errors.Code(422), 422},
		{errors.Code(599), 599},
		{errors.Code(100001), http.StatusOK},
		{errors.Code(0), http.StatusOK},
		{errors.Code(399), http.StatusOK},
		{errors.Code(600), http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("code_%d", tt.code), func(t *testing.T) {
			got := codeToHTTPStatus(tt.code)
			if got != tt.want {
				t.Errorf("codeToHTTPStatus(%d) = %d, want %d", tt.code, got, tt.want)
			}
		})
	}
}
