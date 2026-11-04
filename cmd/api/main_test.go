package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStandardErrorResponse(t *testing.T) {
	r := httptest.NewRecorder()
	fail(r, http.StatusBadRequest, "invalid_sample", "invalid cpu")
	if r.Code != http.StatusBadRequest {
		t.Fatalf("status %d", r.Code)
	}
	if got := r.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type %q", got)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "invalid_sample" || body.Error.Message != "invalid cpu" {
		t.Fatalf("error response: %+v", body)
	}
}

func TestRequestLoggerPreservesHandler(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("X-Test", "yes"); w.WriteHeader(202) })
	for _, path := range []string{"/", "/api/v1/metrics/query"} {
		w := httptest.NewRecorder()
		requestLog(next).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 202 || w.Header().Get("X-Test") != "yes" {
			t.Fatalf("%s: status/header changed", path)
		}
	}
}
