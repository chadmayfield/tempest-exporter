package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func newTestClient() (*Client, *Collector) {
	c := NewCollector("12345", "backyard")
	client := NewClient("test-token", "12345", c)
	return client, c
}

func TestHandleObsST_Valid(t *testing.T) {
	client, collector := newTestClient()

	msg := ObsSTMessage{
		Type: "obs_st",
		Obs: [][]any{{
			float64(1700000000), // timestamp
			float64(0.5),        // wind lull
			float64(1.2),        // wind avg
			float64(2.3),        // wind gust
			float64(180),        // wind direction
			float64(3),          // wind sample interval
			float64(1013.25),    // station pressure
			float64(22.5),       // air temperature
			float64(65.0),       // relative humidity
			float64(50000),      // illuminance
			float64(3.5),        // UV
			float64(300),        // solar radiation
			float64(0.1),        // rain accumulated
			float64(1),          // precipitation type
			float64(10),         // lightning strike avg distance
			float64(2),          // lightning strike count
			float64(2.65),       // battery
			float64(60),         // report interval
		}},
	}
	data, _ := json.Marshal(msg)
	client.handleObsST(data)

	if !collector.HasObservation() {
		t.Fatal("expected observation to be stored")
	}

	collector.mu.RLock()
	obs := collector.obs
	collector.mu.RUnlock()

	if obs.AirTemperature != 22.5 {
		t.Errorf("AirTemperature = %v, want 22.5", obs.AirTemperature)
	}
	if obs.Timestamp != 1700000000 {
		t.Errorf("Timestamp = %d, want 1700000000", obs.Timestamp)
	}
}

func TestHandleObsST_InvalidJSON(t *testing.T) {
	client, collector := newTestClient()
	client.handleObsST([]byte(`{invalid json`))

	if collector.HasObservation() {
		t.Fatal("should not store observation on invalid JSON")
	}
}

func TestHandleObsST_EmptyObs(t *testing.T) {
	client, collector := newTestClient()

	msg := ObsSTMessage{Type: "obs_st", Obs: [][]any{}}
	data, _ := json.Marshal(msg)
	client.handleObsST(data)

	if collector.HasObservation() {
		t.Fatal("should not store observation on empty obs array")
	}
}

func TestHandleObsST_ShortArray(t *testing.T) {
	client, collector := newTestClient()

	msg := ObsSTMessage{
		Type: "obs_st",
		Obs:  [][]any{{float64(1700000000), float64(1.0)}},
	}
	data, _ := json.Marshal(msg)
	client.handleObsST(data)

	if collector.HasObservation() {
		t.Fatal("should not store observation on short array")
	}
}

func TestHandleStrike_Valid(t *testing.T) {
	client, _ := newTestClient()

	msg := StrikeEvent{
		Type: "evt_strike",
		Evt:  []any{float64(1700000000), float64(15.5), float64(100)},
	}
	data, _ := json.Marshal(msg)

	// Should not panic; verifies the handler processes correctly
	client.handleStrike(data)
}

func TestHandleStrike_InvalidJSON(t *testing.T) {
	client, _ := newTestClient()
	// Should not panic
	client.handleStrike([]byte(`not json`))
}

func TestHandleStrike_ShortEvt(t *testing.T) {
	client, _ := newTestClient()

	msg := StrikeEvent{
		Type: "evt_strike",
		Evt:  []any{float64(1700000000)}, // only 1 element, need >= 3
	}
	data, _ := json.Marshal(msg)
	// Should not panic
	client.handleStrike(data)
}

func TestHandleStrike_NullDistance(t *testing.T) {
	client, _ := newTestClient()

	// Manually build JSON with null distance
	data := []byte(`{"type":"evt_strike","evt":[1700000000,null,100]}`)
	// Should not panic — null distance becomes NaN, so the log branch is skipped
	client.handleStrike(data)
}

func TestHandlePrecip_Valid(t *testing.T) {
	client, collector := newTestClient()

	msg := PrecipEvent{
		Type: "evt_precip",
		Evt:  []any{float64(1700000000)},
	}
	data, _ := json.Marshal(msg)
	client.handlePrecip(data)

	collector.mu.RLock()
	rainStart := collector.rainStart
	collector.mu.RUnlock()

	if rainStart != 1700000000 {
		t.Errorf("rainStart = %v, want 1700000000", rainStart)
	}
}

func TestHandlePrecip_InvalidJSON(t *testing.T) {
	client, _ := newTestClient()
	// Should not panic
	client.handlePrecip([]byte(`garbage`))
}

func TestHandlePrecip_EmptyEvt(t *testing.T) {
	client, collector := newTestClient()

	msg := PrecipEvent{Type: "evt_precip", Evt: []any{}}
	data, _ := json.Marshal(msg)
	client.handlePrecip(data)

	collector.mu.RLock()
	rainStart := collector.rainStart
	collector.mu.RUnlock()

	if rainStart != 0 {
		t.Errorf("rainStart should be 0 for empty evt, got %v", rainStart)
	}
}

func TestHandlePrecip_NullEpoch(t *testing.T) {
	client, collector := newTestClient()

	data := []byte(`{"type":"evt_precip","evt":[null]}`)
	client.handlePrecip(data)

	collector.mu.RLock()
	rainStart := collector.rainStart
	collector.mu.RUnlock()

	if rainStart != 0 {
		t.Errorf("rainStart should be 0 for null epoch, got %v", rainStart)
	}
}

func TestNewClient(t *testing.T) {
	c := NewCollector("12345", "backyard")
	client := NewClient("token", "device", c)

	if client.token != "token" {
		t.Errorf("token = %q, want %q", client.token, "token")
	}
	if client.deviceID != "device" {
		t.Errorf("deviceID = %q, want %q", client.deviceID, "device")
	}
	if client.collector != c {
		t.Error("collector not set correctly")
	}
}

func TestRedactToken_InError(t *testing.T) {
	// Simulate what happens when a dial error contains the token
	token := "abc123secret"
	errMsg := "dial wss://ws.weatherflow.com/swd/data?token=" + token + ": connection refused"
	redacted := redactToken(errMsg, token)

	if redacted != "dial wss://ws.weatherflow.com/swd/data?token=[REDACTED]: connection refused" {
		t.Errorf("token not redacted: %s", redacted)
	}
}

func TestParseErrors_RateLimiting(t *testing.T) {
	client, _ := newTestClient()

	// Verify the counter starts at 0
	if client.parseErrors.Load() != 0 {
		t.Fatalf("parseErrors should start at 0, got %d", client.parseErrors.Load())
	}

	// Simulate incrementing parse errors
	client.parseErrors.Add(1)
	if client.parseErrors.Load() != 1 {
		t.Errorf("parseErrors = %d, want 1", client.parseErrors.Load())
	}
}

func TestHandleObsST_NullFields(t *testing.T) {
	client, collector := newTestClient()

	// Build JSON with null fields in the obs array
	data := []byte(`{"type":"obs_st","obs":[[1700000000,null,1.2,null,180,3,1013.25,22.5,65,50000,3.5,300,0.1,1,null,null,2.65,60]]}`)
	client.handleObsST(data)

	if !collector.HasObservation() {
		t.Fatal("expected observation to be stored even with null fields")
	}

	collector.mu.RLock()
	obs := collector.obs
	collector.mu.RUnlock()

	if !math.IsNaN(obs.WindLull) {
		t.Errorf("WindLull = %v, want NaN for null", obs.WindLull)
	}
	if obs.WindAvg != 1.2 {
		t.Errorf("WindAvg = %v, want 1.2", obs.WindAvg)
	}
}

// mockWSServer creates a test WebSocket server that accepts connections,
// reads the listen_start message, sends the provided messages, then closes.
func mockWSServer(_ *testing.T, messages []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		// Read listen_start
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}

		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}
		if msg["type"] != "listen_start" {
			return
		}

		// Send ack
		ack := `{"type":"ack","id":"tempest-exporter"}`
		if err := conn.Write(r.Context(), websocket.MessageText, []byte(ack)); err != nil {
			return
		}

		// Send each test message
		for _, m := range messages {
			if err := conn.Write(r.Context(), websocket.MessageText, []byte(m)); err != nil {
				return
			}
		}

		// Close the connection gracefully
		_ = conn.Close(websocket.StatusNormalClosure, "done")
	}))
}

func TestConnectAndRead_ObsST(t *testing.T) {
	obsMsg := `{"type":"obs_st","obs":[[1700000000,0.5,1.2,2.3,180,3,1013.25,22.5,65,50000,3.5,300,0.1,1,10,2,2.65,60]]}`
	srv := mockWSServer(t, []string{obsMsg})
	defer srv.Close()

	collector := NewCollector("12345", "backyard")
	client := NewClient("test-token", "12345", collector)
	client.wsURL = "ws" + strings.TrimPrefix(srv.URL, "http")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// connectAndRead dials, sends listen_start, and reads until server closes
	err := client.connectAndRead(ctx)
	// Server closes the connection after sending messages, so we expect an error
	if err == nil {
		t.Log("connectAndRead exited without error")
	}

	if !collector.HasObservation() {
		t.Error("expected observation to be stored")
	}

	collector.mu.RLock()
	obs := collector.obs
	collector.mu.RUnlock()

	if obs.AirTemperature != 22.5 {
		t.Errorf("AirTemperature = %v, want 22.5", obs.AirTemperature)
	}
}

func TestConnectAndRead_MultipleMessageTypes(t *testing.T) {
	messages := []string{
		`{"type":"connection_opened"}`,
		`{"type":"obs_st","obs":[[1700000000,0.5,1.2,2.3,180,3,1013.25,22.5,65,50000,3.5,300,0.1,1,10,2,2.65,60]]}`,
		`{"type":"evt_precip","evt":[1700000500]}`,
		`{"type":"evt_strike","evt":[1700000600,15.5,100]}`,
		`{"type":"unknown_type"}`,
	}
	srv := mockWSServer(t, messages)
	defer srv.Close()

	collector := NewCollector("12345", "backyard")
	client := NewClient("test-token", "12345", collector)
	client.wsURL = "ws" + strings.TrimPrefix(srv.URL, "http")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = client.connectAndRead(ctx)

	if !collector.HasObservation() {
		t.Error("expected observation from obs_st message")
	}

	collector.mu.RLock()
	rainStart := collector.rainStart
	collector.mu.RUnlock()

	if rainStart != 1700000500 {
		t.Errorf("rainStart = %v, want 1700000500", rainStart)
	}
}

func TestConnectAndRead_InvalidDeviceID(t *testing.T) {
	srv := mockWSServer(t, nil)
	defer srv.Close()

	collector := NewCollector("12345", "backyard")
	client := NewClient("test-token", "not-a-number", collector)
	client.wsURL = "ws" + strings.TrimPrefix(srv.URL, "http")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.connectAndRead(ctx)
	if err == nil {
		t.Fatal("expected error for invalid device_id")
	}
	if !strings.Contains(err.Error(), "invalid device_id") {
		t.Errorf("expected 'invalid device_id' in error, got: %v", err)
	}
}

func TestConnectAndRead_DialFailure(t *testing.T) {
	collector := NewCollector("12345", "backyard")
	client := NewClient("test-token", "12345", collector)
	client.wsURL = "ws://127.0.0.1:1" // unreachable port

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.connectAndRead(ctx)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	// Verify the token is redacted in the error
	if strings.Contains(err.Error(), "test-token") {
		t.Errorf("token leaked in error: %s", err.Error())
	}
}

func TestRun_ReconnectsAndExitsOnCancel(t *testing.T) {
	// Server that immediately closes each connection
	srv := mockWSServer(t, nil)
	defer srv.Close()

	collector := NewCollector("12345", "backyard")
	client := NewClient("test-token", "12345", collector)
	client.wsURL = "ws" + strings.TrimPrefix(srv.URL, "http")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		client.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Run exited cleanly after context cancellation
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after context cancellation")
	}

	// Should have reconnected at least once
	collector.mu.RLock()
	reconnects := collector.reconnects
	collector.mu.RUnlock()

	if reconnects == 0 {
		t.Error("expected at least one reconnect attempt")
	}
}

func TestRun_BackoffIncreases(t *testing.T) {
	// Server that always closes immediately — forces multiple reconnects.
	// Each connection is accepted then closed, driving the backoff loop.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		// Read listen_start then close
		_, _, _ = conn.Read(r.Context())
		_ = conn.Close(websocket.StatusNormalClosure, "bye")
	}))
	defer srv.Close()

	collector := NewCollector("12345", "backyard")
	client := NewClient("test-token", "12345", collector)
	client.wsURL = "ws" + strings.TrimPrefix(srv.URL, "http")

	// Run for 3.5s — should hit: connect, 1s wait, connect, 2s wait, then cancel
	ctx, cancel := context.WithTimeout(context.Background(), 3500*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		client.Run(ctx)
		close(done)
	}()

	<-done

	collector.mu.RLock()
	reconnects := collector.reconnects
	collector.mu.RUnlock()

	// With 1s + 2s backoff = 3s of waiting, should get at least 2 reconnects
	if reconnects < 2 {
		t.Errorf("expected >= 2 reconnects, got %v", reconnects)
	}
}

func TestConnectAndRead_WriteError(t *testing.T) {
	// Server that accepts the WebSocket but closes immediately,
	// causing the client's Write (listen_start) to fail.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		// Close immediately — don't read listen_start
		_ = conn.Close(websocket.StatusGoingAway, "bye")
	}))
	defer srv.Close()

	collector := NewCollector("12345", "backyard")
	client := NewClient("test-token", "12345", collector)
	client.wsURL = "ws" + strings.TrimPrefix(srv.URL, "http")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.connectAndRead(ctx)
	if err == nil {
		t.Fatal("expected error when server closes before write")
	}
	if strings.Contains(err.Error(), "test-token") {
		t.Errorf("token leaked in error: %s", err.Error())
	}
}

func TestReadLoop_UnparseableMessages(t *testing.T) {
	messages := []string{
		`not json at all`,
		`also {{{ invalid`,
		`{"type":"obs_st","obs":[[1700000000,0.5,1.2,2.3,180,3,1013.25,22.5,65,50000,3.5,300,0.1,1,10,2,2.65,60]]}`,
	}
	srv := mockWSServer(t, messages)
	defer srv.Close()

	collector := NewCollector("12345", "backyard")
	client := NewClient("test-token", "12345", collector)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	wsTestURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	wsConn, _, err := websocket.Dial(ctx, wsTestURL+"?token=test-token", nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer func() { _ = wsConn.CloseNow() }()

	listenMsg := map[string]any{"type": "listen_start", "device_id": 12345, "id": "tempest-exporter"}
	data, _ := json.Marshal(listenMsg)
	_ = wsConn.Write(ctx, websocket.MessageText, data)

	_ = client.readLoop(ctx, wsConn)

	// Despite unparseable messages, the valid obs_st should still be processed
	if !collector.HasObservation() {
		t.Error("expected observation despite earlier parse errors")
	}

	// Parse error counter should reflect the bad messages
	if client.parseErrors.Load() < 2 {
		t.Errorf("parseErrors = %d, want >= 2", client.parseErrors.Load())
	}
}
