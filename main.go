package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var version = "dev"

// validStationName matches alphanumeric, underscore, hyphen, and dot characters only.
var validStationName = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	// Default to JSON logging; use text for local dev via LOG_FORMAT=text
	if strings.EqualFold(os.Getenv("LOG_FORMAT"), "text") {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	} else {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	}

	token := os.Getenv("TEMPEST_TOKEN")
	deviceID := os.Getenv("TEMPEST_DEVICE_ID")
	stationID := os.Getenv("TEMPEST_STATION_ID")

	if token == "" || deviceID == "" || stationID == "" {
		slog.Error("missing required environment variables",
			"required", "TEMPEST_TOKEN, TEMPEST_DEVICE_ID, TEMPEST_STATION_ID")
		os.Exit(1)
	}

	stationName := os.Getenv("TEMPEST_STATION_NAME")
	if stationName == "" {
		stationName = "tempest"
	}
	if !validStationName.MatchString(stationName) {
		slog.Error("invalid TEMPEST_STATION_NAME: must contain only alphanumeric, underscore, hyphen, or dot characters",
			"value", stationName)
		os.Exit(1)
	}

	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":8080"
	}
	if _, _, err := net.SplitHostPort(listenAddr); err != nil {
		slog.Error("invalid LISTEN_ADDR", "value", listenAddr, "error", err)
		os.Exit(1)
	}

	slog.Info("starting tempest-exporter",
		"version", version,
		"listen_addr", listenAddr,
		"device_id", deviceID,
		"station_id", stationID,
		"station_name", stationName,
	)

	collector := NewCollector(stationID, stationName)
	prometheus.MustRegister(collector)

	wsClient := NewClient(token, deviceID, collector)
	restClient := NewRESTClient(token, stationID, collector)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start WebSocket client
	go wsClient.Run(ctx)

	// Start REST fallback (activates after 5min disconnect, polls every 60s)
	go restClient.RunFallback(ctx, 5*time.Minute, 60*time.Second)

	mux := newMux(collector)

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig.String())
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
	}()

	slog.Info("HTTP server listening", "addr", listenAddr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("HTTP server error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

// newMux creates the HTTP handler with /metrics, /healthz, and /readyz endpoints.
func newMux(collector *Collector) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if collector.HasObservation() {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintln(w, "ready")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintln(w, "not ready: no observations received")
		}
	})
	return mux
}
