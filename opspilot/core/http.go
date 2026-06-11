package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/audit"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/response"
)

type handlerFunc func(context.Context, *http.Request) (any, []string, error)

var httpAuditRecorder = audit.NewRecorder("")

func setAuditRecorder(recorder *audit.Recorder) {
	if recorder == nil {
		httpAuditRecorder = audit.NewRecorder("")
		return
	}
	httpAuditRecorder = recorder
}

func handleAPI(mux *http.ServeMux, path string, handler http.HandlerFunc) {
	mux.HandleFunc(path, handler)
	if versioned := apiV1Path(path); versioned != path {
		mux.HandleFunc(versioned, handler)
	}
}

func apiV1Path(path string) string {
	if !strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/api/v1/") {
		return path
	}
	return "/api/v1/" + strings.TrimPrefix(path, "/api/")
}

func k8sClientForRequest(r *http.Request, registry *k8s.Registry) (*k8s.Client, []string, error) {
	cluster := r.URL.Query().Get("cluster")
	if cluster == "" {
		cluster = r.Form.Get("cluster")
	}
	return registry.ClientFor(cluster)
}

func wrap(fn handlerFunc) http.HandlerFunc {
	return wrapMethod(http.MethodGet, fn)
}

func wrapPost(fn handlerFunc) http.HandlerFunc {
	return wrapMethod(http.MethodPost, fn)
}

func wrapMethod(method string, fn handlerFunc) http.HandlerFunc {
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
			recordHTTPAudit(r, status, "error", err.Error(), nil)
		}()
		if r.Method != method {
			writeJSON(w, http.StatusMethodNotAllowed, response.Error("METHOD_NOT_ALLOWED", "only "+method+" is supported"))
			recordHTTPAudit(r, http.StatusMethodNotAllowed, "error", "method not allowed", nil)
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
			recordHTTPAudit(r, status, "error", err.Error(), warnings)
			return
		}
		writeJSON(w, http.StatusOK, response.OK(data, warnings))
		recordHTTPAudit(r, http.StatusOK, "success", "", warnings)
	}
}

func recordHTTPAudit(r *http.Request, status int, outcome, errMessage string, warnings []string) {
	if httpAuditRecorder == nil || !httpAuditRecorder.Enabled() {
		return
	}
	record := audit.Record{
		Actor:      actorFromRequest(r),
		Method:     r.Method,
		Path:       r.URL.Path,
		Action:     actionFromPath(r.URL.Path),
		TargetType: targetTypeFromRequest(r),
		Target:     targetFromRequest(r),
		Namespace:  firstQueryValue(r, "namespace", "n"),
		Cluster:    firstQueryValue(r, "cluster"),
		Risk:       audit.RiskFor(r.Method, r.URL.Path),
		Outcome:    outcome,
		StatusCode: status,
		Error:      errMessage,
		Warnings:   warnings,
		Query:      safeQuery(r),
	}
	if err := httpAuditRecorder.Record(record); err != nil {
		fmt.Fprintf(os.Stderr, "audit_write_failed path=%s error=%v\n", r.URL.Path, err)
	}
}

func actorFromRequest(r *http.Request) string {
	for _, name := range []string{"X-OpsPilot-Actor", "X-Forwarded-User", "X-Remote-User"} {
		if value := strings.TrimSpace(r.Header.Get(name)); value != "" {
			return value
		}
	}
	return "anonymous"
}

func actionFromPath(path string) string {
	path = strings.TrimPrefix(path, "/api/v1/")
	path = strings.TrimPrefix(path, "/api/")
	return strings.ReplaceAll(path, "/", " ")
}

func targetTypeFromRequest(r *http.Request) string {
	if value := firstQueryValue(r, "target_type"); value != "" {
		return value
	}
	path := strings.ToLower(r.URL.Path)
	switch {
	case strings.Contains(path, "host"):
		return "host"
	case strings.Contains(path, "pod"):
		return "pod"
	case strings.Contains(path, "release") || strings.Contains(path, "service"):
		return "service"
	case strings.Contains(path, "cluster"):
		return "cluster"
	default:
		return ""
	}
}

func targetFromRequest(r *http.Request) string {
	return firstQueryValue(r, "service", "pod", "host", "name", "target")
}

func firstQueryValue(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(r.URL.Query().Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func safeQuery(r *http.Request) map[string]string {
	out := map[string]string{}
	for key, values := range r.URL.Query() {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") {
			out[key] = "[redacted]"
			continue
		}
		if len(values) > 0 {
			out[key] = values[len(values)-1]
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

func int64Query(r *http.Request, name string, fallback int64) int64 {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
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

func queryList(r *http.Request, name string) []string {
	values := []string{}
	for _, raw := range r.URL.Query()[name] {
		for _, part := range strings.FieldsFunc(raw, func(ch rune) bool {
			return ch == ',' || ch == '|'
		}) {
			part = strings.TrimSpace(part)
			if part != "" {
				values = append(values, part)
			}
		}
	}
	return values
}

func boolForm(r *http.Request, name string) bool {
	raw := r.Form.Get(name)
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func releaseVariablesFromForm(r *http.Request) map[string]string {
	variables := map[string]string{}
	for key, values := range r.Form {
		if !strings.HasPrefix(key, "var.") || len(values) == 0 {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(key, "var."))
		if name == "" {
			continue
		}
		variables[name] = values[len(values)-1]
	}
	return variables
}
