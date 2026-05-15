package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/response"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/version"
)

func main() {
	host := flag.String("host", env("OPSPILOT_HOST", "0.0.0.0"), "listen host")
	port := flag.String("port", env("OPSPILOT_PORT", "18080"), "listen port")
	flag.Parse()

	client := k8s.NewClient()
	mux := http.NewServeMux()
	registerRoutes(mux, client)
	addr := *host + ":" + *port
	fmt.Printf("opspilot-core %s listening on http://%s\n", version.Version, addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func registerRoutes(mux *http.ServeMux, client *k8s.Client) {
	mux.HandleFunc("/api/health", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return map[string]any{"version": version.Version, "kubernetes": client.Health()}, nil, nil
	}))
	mux.HandleFunc("/api/inventory/overview", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return client.InventoryOverview(ctx, intQuery(r, "limit", 10)), nil, nil
	}))
	mux.HandleFunc("/api/k8s/pods", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, err := client.ListPods(ctx, q.Get("namespace"), q.Get("status"), q.Get("q"), intQuery(r, "limit", 100))
		return result, nil, err
	}))
	mux.HandleFunc("/api/k8s/logs/pod", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		req := k8s.LogRequest{
			Namespace:    required(q.Get("namespace"), "namespace"),
			Pod:          required(q.Get("pod"), "pod"),
			Container:    q.Get("container"),
			TailLines:    intQueryAliases(r, []string{"tail_lines", "tail"}, 300),
			SinceSeconds: intQueryAliases(r, []string{"since_seconds", "since"}, 1800),
			LimitBytes:   intQuery(r, "limit_bytes", 1024*1024),
			Previous:     boolQuery(r, "previous"),
			Timestamps:   boolQuery(r, "timestamps"),
		}
		log, err := client.ReadPodLog(ctx, req)
		return log, nil, err
	}))
	mux.HandleFunc("/api/context/pod", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		podContext, err := client.PodContext(ctx, required(q.Get("namespace"), "namespace"), required(q.Get("pod"), "pod"))
		return podContext, nil, err
	}))
	mux.HandleFunc("/api/diagnose/pod", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		diagnosis, err := client.DiagnosePod(ctx, required(q.Get("namespace"), "namespace"), required(q.Get("pod"), "pod"))
		return diagnosis, nil, err
	}))
}

type handlerFunc func(context.Context, *http.Request) (any, []string, error)

func wrap(fn handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			err, ok := rec.(error)
			if !ok {
				err = fmt.Errorf("%v", rec)
			}
			status := http.StatusInternalServerError
			code := "INTERNAL_ERROR"
			if errors.Is(err, errBadRequest) {
				status = http.StatusBadRequest
				code = "BAD_REQUEST"
			}
			writeJSON(w, status, response.Error(code, err.Error()))
		}()
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, response.Error("METHOD_NOT_ALLOWED", "only GET is supported"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		data, warnings, err := fn(ctx, r)
		if err != nil {
			status := http.StatusInternalServerError
			code := "INTERNAL_ERROR"
			if errors.Is(err, errBadRequest) {
				status = http.StatusBadRequest
				code = "BAD_REQUEST"
			}
			writeJSON(w, status, response.Error(code, err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, response.OK(data, warnings))
	}
}

func writeJSON(w http.ResponseWriter, status int, body response.Envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

var errBadRequest = errors.New("bad request")

type requestError struct {
	message string
}

func (e requestError) Error() string {
	return e.message
}

func (e requestError) Is(target error) bool {
	return target == errBadRequest
}

func required(value, name string) string {
	if value == "" {
		panic(requestError{message: name + " is required"})
	}
	return value
}

func intQuery(r *http.Request, name string, fallback int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		panic(requestError{message: name + " must be an integer"})
	}
	return value
}

func intQueryAliases(r *http.Request, names []string, fallback int) int {
	for _, name := range names {
		if r.URL.Query().Get(name) != "" {
			return intQuery(r, name, fallback)
		}
	}
	return fallback
}

func boolQuery(r *http.Request, name string) bool {
	raw := r.URL.Query().Get(name)
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
