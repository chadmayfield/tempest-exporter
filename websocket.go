package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const defaultWSURL = "wss://ws.weatherflow.com/swd/data"

// readTimeout is the maximum time to wait for a single WebSocket message.
// obs_st arrives every ~60s; 5 minutes accommodates network jitter.
const readTimeout = 5 * time.Minute

// Client manages the WebSocket connection to the Tempest API.
type Client struct {
	token    string
	deviceID string
	wsURL    string
	collector *Collector

	// parseErrors tracks consecutive unparseable messages for rate-limited logging.
	parseErrors atomic.Int64
}

// NewClient creates a new WebSocket client.
func NewClient(token, deviceID string, collector *Collector) *Client {
	return &Client{
		token:    token,
		deviceID: deviceID,
		wsURL:    defaultWSURL,
		collector: collector,
	}
}

// Run maintains a persistent WebSocket connection with exponential backoff reconnection.
// It blocks until the context is cancelled.
func (c *Client) Run(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 60 * time.Second

	for {
		start := time.Now()
		err := c.connectAndRead(ctx)
		if ctx.Err() != nil {
			return
		}

		c.collector.SetConnected(false)
		c.collector.IncrReconnects()

		// Reset backoff if the connection was up for a while
		if time.Since(start) > 2*time.Minute {
			backoff = time.Second
		}

		slog.Warn("websocket disconnected",
			"error", err,
			"reconnect_in", backoff,
		)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// connectAndRead dials the WebSocket, sends listen_start, and reads messages.
// Returns on error or context cancellation.
func (c *Client) connectAndRead(ctx context.Context) error {
	dialOpts := &websocket.DialOptions{
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
		},
	}

	url := fmt.Sprintf("%s?token=%s", c.wsURL, c.token)
	conn, _, err := websocket.Dial(ctx, url, dialOpts)
	if err != nil {
		// Do not wrap the dial error directly — it may contain the URL with the token.
		return fmt.Errorf("websocket dial failed: %v", redactToken(err.Error(), c.token))
	}
	defer func() { _ = conn.CloseNow() }()

	// Send listen_start to subscribe to device observations.
	// device_id must be a number per the WeatherFlow API spec.
	deviceIDNum, err := strconv.Atoi(c.deviceID)
	if err != nil {
		return fmt.Errorf("invalid device_id %q: %w", c.deviceID, err)
	}
	listenMsg := map[string]any{
		"type":      "listen_start",
		"device_id": deviceIDNum,
		"id":        "tempest-exporter",
	}
	data, err := json.Marshal(listenMsg)
	if err != nil {
		return fmt.Errorf("marshal listen_start: %w", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("send listen_start: %v", redactToken(err.Error(), c.token))
	}

	c.collector.SetConnected(true)
	slog.Info("websocket connected", "device_id", c.deviceID)

	c.parseErrors.Store(0)
	return c.readLoop(ctx, conn)
}

// readLoop reads and dispatches WebSocket messages until error or context cancellation.
func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		// Apply a per-message read timeout so we don't block forever
		// if the server stops sending data.
		readCtx, readCancel := context.WithTimeout(ctx, readTimeout)
		_, data, err := conn.Read(readCtx)
		readCancel()
		if err != nil {
			return fmt.Errorf("read: %v", redactToken(err.Error(), c.token))
		}

		var envelope WSMessage
		if err := json.Unmarshal(data, &envelope); err != nil {
			count := c.parseErrors.Add(1)
			// Rate-limit: log first occurrence, then every 100th
			if count == 1 || count%100 == 0 {
				slog.Warn("ignoring unparseable message",
					"error", err,
					"total_parse_errors", count,
				)
			}
			continue
		}

		switch envelope.Type {
		case "obs_st":
			c.handleObsST(data)
		case "evt_strike":
			c.handleStrike(data)
		case "evt_precip":
			c.handlePrecip(data)
		case "ack", "connection_opened":
			slog.Info("received control message", "type", envelope.Type)
		default:
			slog.Warn("ignoring unknown message type", "type", envelope.Type)
		}
	}
}

func (c *Client) handleObsST(data []byte) {
	var msg ObsSTMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Error("error parsing obs_st", "error", err)
		return
	}
	if len(msg.Obs) == 0 {
		slog.Warn("obs_st with empty obs array")
		return
	}
	obs, err := ParseObservation(msg.Obs[0])
	if err != nil {
		slog.Error("error parsing observation", "error", err)
		return
	}
	c.collector.UpdateObservation(obs)
	slog.Info("observation updated",
		"air_temp_c", obs.AirTemperature,
		"humidity_pct", obs.RelativeHumidity,
		"wind_mps", obs.WindAvg,
		"pressure_mb", obs.StationPressure,
	)
}

func (c *Client) handleStrike(data []byte) {
	var msg StrikeEvent
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Error("error parsing evt_strike", "error", err)
		return
	}
	if len(msg.Evt) >= 3 {
		dist := toFloat(msg.Evt[1])
		energy := toFloat(msg.Evt[2])
		if !math.IsNaN(dist) {
			slog.Info("lightning strike detected",
				"distance_km", dist,
				"energy", energy,
			)
		}
	}
}

func (c *Client) handlePrecip(data []byte) {
	var msg PrecipEvent
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Error("error parsing evt_precip", "error", err)
		return
	}
	if len(msg.Evt) >= 1 {
		epoch := toFloat(msg.Evt[0])
		if !math.IsNaN(epoch) {
			c.collector.SetRainStart(epoch)
			slog.Info("rain start event", "epoch", epoch)
		}
	}
}
