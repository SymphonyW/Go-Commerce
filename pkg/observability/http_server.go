package observability

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

type ReadinessCheck func(context.Context) error

func NewProbeMux(metricsHandler http.Handler, checks map[string]ReadinessCheck) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", metricsHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		failures := make(map[string]string)
		for name, check := range checks {
			if check == nil {
				continue
			}
			if err := check(r.Context()); err != nil {
				failures[name] = err.Error()
			}
		}
		if len(failures) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":   "not_ready",
				"failures": failures,
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})
	return mux
}

func StartHTTPServer(addr string, handler http.Handler, logger *slog.Logger) *http.Server {
	if logger == nil {
		logger = slog.Default()
	}
	server := &http.Server{Addr: addr, Handler: handler}
	go func() {
		logger.Info("observability_http_server_started", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("observability_http_server_failed", "addr", addr, "error", err)
		}
	}()
	return server
}
