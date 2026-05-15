package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequiredPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	required("", "namespace")
}

func TestWrapConvertsBadRequestPanic(t *testing.T) {
	handler := wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		required("", "namespace")
		return nil, nil, nil
	})
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/test", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "namespace is required") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}
