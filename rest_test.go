package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const fixtureJSON = `{
	"obs": [{
		"timestamp": 1700000000,
		"wind_lull": 0.5,
		"wind_avg": 1.2,
		"wind_gust": 2.3,
		"wind_direction": 180,
		"station_pressure": 1013.25,
		"air_temperature": 22.5,
		"relative_humidity": 65.0,
		"illuminance": 50000,
		"uv": 3.5,
		"solar_radiation": 300,
		"rain_accumulated": 0.1,
		"precip_type": 1,
		"lightning_strike_avg_distance": 10,
		"lightning_strike_count": 2,
		"battery": 2.65,
		"report_interval": 60
	}]
}`

func TestRESTClient_FetchObservation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/observations/station/99999" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("token") != "test-token" {
			t.Errorf("unexpected token: %s", r.URL.Query().Get("token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixtureJSON))
	}))
	defer srv.Close()

	c := NewCollector("99999", "test")
	rc := NewRESTClient("test-token", "99999", c)
	rc.baseURL = srv.URL

	obs, err := rc.FetchObservation(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if obs.Timestamp != 1700000000 {
		t.Errorf("Timestamp = %d, want 1700000000", obs.Timestamp)
	}
	if obs.AirTemperature != 22.5 {
		t.Errorf("AirTemperature = %v, want 22.5", obs.AirTemperature)
	}
	if obs.Battery != 2.65 {
		t.Errorf("Battery = %v, want 2.65", obs.Battery)
	}
	if obs.StationPressure != 1013.25 {
		t.Errorf("StationPressure = %v, want 1013.25", obs.StationPressure)
	}
}

func TestRESTClient_FetchObservation_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":{"status_code":401,"status_message":"UNAUTHORIZED"}}`))
	}))
	defer srv.Close()

	c := NewCollector("99999", "test")
	rc := NewRESTClient("bad-token", "99999", c)
	rc.baseURL = srv.URL

	_, err := rc.FetchObservation(context.Background())
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestRESTClient_FetchObservation_EmptyObs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"obs":[]}`))
	}))
	defer srv.Close()

	c := NewCollector("99999", "test")
	rc := NewRESTClient("test-token", "99999", c)
	rc.baseURL = srv.URL

	_, err := rc.FetchObservation(context.Background())
	if err == nil {
		t.Fatal("expected error for empty obs")
	}
}

func TestRESTClient_FetchObservation_NullFields(t *testing.T) {
	nullJSON := `{
		"obs": [{
			"timestamp": 1700000000,
			"air_temperature": 22.5,
			"relative_humidity": null,
			"wind_avg": null,
			"battery": 2.65
		}]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(nullJSON))
	}))
	defer srv.Close()

	c := NewCollector("99999", "test")
	rc := NewRESTClient("test-token", "99999", c)
	rc.baseURL = srv.URL

	obs, err := rc.FetchObservation(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if obs.AirTemperature != 22.5 {
		t.Errorf("AirTemperature = %v, want 22.5", obs.AirTemperature)
	}
	// Null fields should be zero
	if obs.RelativeHumidity != 0 {
		t.Errorf("RelativeHumidity = %v, want 0 (null)", obs.RelativeHumidity)
	}
	if obs.WindAvg != 0 {
		t.Errorf("WindAvg = %v, want 0 (null)", obs.WindAvg)
	}
}

func TestRESTClient_FetchObservation_RedactsToken(t *testing.T) {
	// Use an unreachable address to trigger a connection error containing the URL.
	c := NewCollector("99999", "test")
	rc := NewRESTClient("supersecrettoken", "99999", c)
	rc.baseURL = "http://127.0.0.1:1" // unreachable port

	_, err := rc.FetchObservation(context.Background())
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if strings.Contains(err.Error(), "supersecrettoken") {
		t.Errorf("token leaked in error: %s", err.Error())
	}
}

func TestRESTClient_FetchObservation_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not valid json}`))
	}))
	defer srv.Close()

	c := NewCollector("99999", "test")
	rc := NewRESTClient("test-token", "99999", c)
	rc.baseURL = srv.URL

	_, err := rc.FetchObservation(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestRunFallback_PollsWhenDisconnected(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixtureJSON))
	}))
	defer srv.Close()

	c := NewCollector("99999", "test")
	// Leave c.connected as false (default) to simulate disconnected state
	rc := NewRESTClient("test-token", "99999", c)
	rc.baseURL = srv.URL

	ctx, cancel := context.WithCancel(context.Background())

	// Run fallback with very short thresholds for testing:
	// 0 disconnect threshold = activate immediately, 50ms poll interval
	done := make(chan struct{})
	go func() {
		rc.RunFallback(ctx, 0, 50*time.Millisecond)
		close(done)
	}()

	// Wait enough for a couple of poll cycles
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	if requestCount == 0 {
		t.Error("RunFallback did not poll the REST API")
	}
	if !c.HasObservation() {
		t.Error("RunFallback should have stored an observation")
	}
}

func TestRunFallback_SkipsWhenConnected(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixtureJSON))
	}))
	defer srv.Close()

	c := NewCollector("99999", "test")
	c.SetConnected(true) // WebSocket is connected

	rc := NewRESTClient("test-token", "99999", c)
	rc.baseURL = srv.URL

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		rc.RunFallback(ctx, 0, 50*time.Millisecond)
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	if requestCount != 0 {
		t.Errorf("RunFallback should not poll when connected, got %d requests", requestCount)
	}
}

func TestRunFallback_CancelsCleanly(t *testing.T) {
	c := NewCollector("99999", "test")
	rc := NewRESTClient("test-token", "99999", c)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	done := make(chan struct{})
	go func() {
		rc.RunFallback(ctx, 5*time.Minute, 60*time.Second)
		close(done)
	}()

	select {
	case <-done:
		// success — function exited cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("RunFallback did not exit after context cancellation")
	}
}

func TestIsConnected(t *testing.T) {
	c := NewCollector("12345", "test")

	if c.isConnected() {
		t.Error("isConnected should be false by default")
	}

	c.SetConnected(true)
	if !c.isConnected() {
		t.Error("isConnected should be true after SetConnected(true)")
	}

	c.SetConnected(false)
	if c.isConnected() {
		t.Error("isConnected should be false after SetConnected(false)")
	}
}
