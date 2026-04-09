package storage

import (
	"errors"
	"fmt"
	"testing"

	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
)

func TestWrapTOSError(t *testing.T) {
	// newServerErr constructs a *tos.TosServerError with the given Code and
	// StatusCode. The SDK's tos.Code / tos.StatusCode helpers read these
	// fields via type assertion, which is exactly what wrapTOSError relies on.
	newServerErr := func(code string, status int) error {
		return &tos.TosServerError{
			Code:        code,
			RequestInfo: tos.RequestInfo{StatusCode: status},
		}
	}

	tests := []struct {
		name    string
		in      error
		wantErr error // sentinel expected via errors.Is; nil means "unchanged"
	}{
		{
			name:    "nil passes through",
			in:      nil,
			wantErr: nil,
		},
		{
			name:    "NoSuchKey maps to ErrNotFound",
			in:      newServerErr("NoSuchKey", 404),
			wantErr: ErrNotFound,
		},
		{
			name:    "NoSuchBucket maps to ErrNotFound",
			in:      newServerErr("NoSuchBucket", 404),
			wantErr: ErrNotFound,
		},
		{
			name:    "AccessDenied maps to ErrAccessDenied",
			in:      newServerErr("AccessDenied", 403),
			wantErr: ErrAccessDenied,
		},
		{
			name:    "404 without code falls back to ErrNotFound",
			in:      newServerErr("", 404),
			wantErr: ErrNotFound,
		},
		{
			name:    "403 without code falls back to ErrAccessDenied",
			in:      newServerErr("", 403),
			wantErr: ErrAccessDenied,
		},
		{
			// SignatureDoesNotMatch is also 403 but should NOT be classified as
			// AccessDenied — Code takes precedence, and since the code is not in
			// our switch, it falls through to the status-code branch. Document
			// this known limitation: a 403 we can't name-match still becomes
			// ErrAccessDenied. That's acceptable — callers treating it as
			// access-denied is better than losing the signal entirely.
			name:    "SignatureDoesNotMatch 403 still classified as AccessDenied",
			in:      newServerErr("SignatureDoesNotMatch", 403),
			wantErr: ErrAccessDenied,
		},
		{
			name:    "unrelated error returned unchanged",
			in:      errors.New("some transient network blip"),
			wantErr: nil, // unchanged
		},
		{
			// Guard against the old string-matching bug: an error whose message
			// contains "404" (e.g. a key literally named "file404") must not be
			// misclassified as ErrNotFound.
			name:    "unrelated error containing 404 substring is NOT classified",
			in:      fmt.Errorf("failed to process key \"uploads/file404.jpg\""),
			wantErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapTOSError(tc.in)

			if tc.in == nil {
				if got != nil {
					t.Fatalf("wrapTOSError(nil) = %v, want nil", got)
				}
				return
			}

			if tc.wantErr == nil {
				// "unchanged" means the original error is returned as-is.
				if got != tc.in {
					t.Fatalf("wrapTOSError(%v) = %v, want unchanged", tc.in, got)
				}
				return
			}

			if !errors.Is(got, tc.wantErr) {
				t.Fatalf("wrapTOSError(%v) = %v, want errors.Is(_, %v)", tc.in, got, tc.wantErr)
			}
		})
	}
}
