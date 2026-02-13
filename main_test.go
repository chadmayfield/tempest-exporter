package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthz(t *testing.T) {
	c := NewCollector("99999", "test")
	mux := newMux(c)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Errorf("healthz body = %q, want 'ok'", w.Body.String())
	}
}

func TestReadyz_NotReady(t *testing.T) {
	c := NewCollector("99999", "test")
	mux := newMux(c)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz status = %d, want 503", w.Code)
	}
}

func TestReadyz_Ready(t *testing.T) {
	c := NewCollector("99999", "test")
	obs := Observation{Timestamp: 1700000000, AirTemperature: 22.5}
	c.UpdateObservation(obs)
	mux := newMux(c)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("readyz status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ready") {
		t.Errorf("readyz body = %q, want 'ready'", w.Body.String())
	}
}

func TestMetricsEndpoint(t *testing.T) {
	c := NewCollector("99999", "test")
	mux := newMux(c)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("metrics status = %d, want 200", w.Code)
	}
}

func TestValidStationName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"tempest", true},
		{"my-station", true},
		{"station_1", true},
		{"station.name", true},
		{"bad station", false},
		{"bad;injection", false},
		{"", false},
	}
	for _, tt := range tests {
		got := validStationName.MatchString(tt.name)
		if got != tt.valid {
			t.Errorf("validStationName(%q) = %v, want %v", tt.name, got, tt.valid)
		}
	}
}
