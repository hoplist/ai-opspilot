package logsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPISIXIndex          = "apisix-*"
	defaultTimeField            = "@timestamp"
	defaultAPISIXHostField      = "host_02"
	defaultAPISIXURIField       = "uri"
	defaultAPISIXReqField       = "request"
	defaultServiceURIField      = "msg"
	defaultCorrelationLimit     = 20
	maxCorrelationSinceSeconds  = 7200
	maxCorrelationWindowSeconds = 3600
)

type CorrelationConfig struct {
	APISIXIndex   string
	DisableAPISIX bool
	TimeField     string

	APISIXHostField   string
	APISIXURIField    string
	APISIXReqField    string
	APISIXStatusField string

	ServiceIndex    string
	ServiceURIField string

	Routes []CorrelationRoute
}

type CorrelationRoute struct {
	Name                 string `json:"name"`
	Host                 string `json:"host"`
	PathPrefix           string `json:"path_prefix"`
	Service              string `json:"service"`
	ServiceIndex         string `json:"service_index"`
	ServiceURIField      string `json:"service_uri_field"`
	ServiceEventField    string `json:"service_event_field"`
	ServiceEventTemplate string `json:"service_event_template"`
	ServiceFallbackQuery string `json:"service_fallback_query"`
}

type CorrelateRequest struct {
	Host            string
	URI             string
	Status          string
	At              string
	SinceSeconds    int
	WindowSeconds   int
	Limit           int
	IncludeOptions  bool
	SkipAPISIX      bool
	APISIXIndex     string
	ServiceIndex    string
	ServiceURIField string
}

func (c CorrelationConfig) withDefaults() CorrelationConfig {
	if c.APISIXIndex == "" {
		c.APISIXIndex = defaultAPISIXIndex
	}
	if c.TimeField == "" {
		c.TimeField = defaultTimeField
	}
	if c.APISIXHostField == "" {
		c.APISIXHostField = defaultAPISIXHostField
	}
	if c.APISIXURIField == "" {
		c.APISIXURIField = defaultAPISIXURIField
	}
	if c.APISIXReqField == "" {
		c.APISIXReqField = defaultAPISIXReqField
	}
	if c.APISIXStatusField == "" {
		c.APISIXStatusField = "status"
	}
	if c.ServiceURIField == "" {
		c.ServiceURIField = defaultServiceURIField
	}
	for i := range c.Routes {
		if c.Routes[i].ServiceURIField == "" {
			c.Routes[i].ServiceURIField = c.ServiceURIField
		}
	}
	return c
}

// ParseCorrelationRoutes accepts semicolon-separated route rules:
// name|host|path_prefix|service_index|service_uri_field|service_event_field|service_event_template|service_fallback_query
func ParseCorrelationRoutes(raw string) []CorrelationRoute {
	routes := []CorrelationRoute{}
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "|")
		for len(parts) < 8 {
			parts = append(parts, "")
		}
		route := CorrelationRoute{
			Name:                 strings.TrimSpace(parts[0]),
			Host:                 strings.TrimSpace(parts[1]),
			PathPrefix:           strings.TrimSpace(parts[2]),
			ServiceIndex:         strings.TrimSpace(parts[3]),
			ServiceURIField:      strings.TrimSpace(parts[4]),
			ServiceEventField:    strings.TrimSpace(parts[5]),
			ServiceEventTemplate: strings.TrimSpace(parts[6]),
			ServiceFallbackQuery: strings.TrimSpace(parts[7]),
		}
		if route.Name == "" {
			route.Name = route.Host + route.PathPrefix
		}
		if route.Host != "" && route.PathPrefix != "" && route.ServiceIndex != "" {
			routes = append(routes, route)
		}
	}
	return routes
}

func (c *Client) CorrelateRequest(ctx context.Context, req CorrelateRequest) (map[string]any, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("log search is not configured")
	}
	if strings.TrimSpace(req.URI) == "" && strings.TrimSpace(req.Host) == "" && strings.TrimSpace(req.ServiceIndex) == "" {
		return nil, fmt.Errorf("host, uri, or service_index is required")
	}
	limit := clampLimit(req.Limit)
	start, end, mode, err := correlationRange(req)
	if err != nil {
		return nil, err
	}
	config := c.correlation.withDefaults()
	skipAPISIX := req.SkipAPISIX || config.DisableAPISIX || strings.TrimSpace(req.Host) == ""
	apisixIndex := valueOr(req.APISIXIndex, config.APISIXIndex)
	route := c.matchRoute(req.Host, req.URI)
	serviceIndex := config.ServiceIndex
	serviceURIField := config.ServiceURIField
	if route != nil {
		serviceIndex = valueOr(route.ServiceIndex, serviceIndex)
		serviceURIField = valueOr(route.ServiceURIField, serviceURIField)
	}
	serviceIndex = valueOr(req.ServiceIndex, serviceIndex)
	serviceURIField = valueOr(req.ServiceURIField, serviceURIField)

	apisixResult := map[string]any{}
	if skipAPISIX {
		reason := "skipped by request or config"
		if strings.TrimSpace(req.Host) == "" {
			reason = "host is empty; APISIX lookup requires host"
		}
		apisixResult = map[string]any{
			"status": "skipped",
			"index":  apisixIndex,
			"reason": reason,
		}
	} else {
		var err error
		apisixResult, err = c.searchAPISIX(ctx, config, apisixIndex, req, start, end, limit)
		if err != nil {
			apisixResult = map[string]any{
				"status": "error",
				"index":  apisixIndex,
				"error":  err.Error(),
			}
		}
	}
	serviceResult := map[string]any{
		"status": "skipped",
		"reason": "service_index is not configured or provided",
	}
	if serviceIndex != "" {
		serviceResult, err = c.searchServiceLogs(ctx, config, route, serviceIndex, serviceURIField, req.URI, req.Status, start, end, limit)
		if err != nil {
			serviceResult = map[string]any{
				"status": "error",
				"error":  err.Error(),
			}
		}
	}

	gaps := correlationGaps(apisixResult, serviceResult)
	strength := correlationStrength(apisixResult, serviceResult)
	findings := correlationFindings(apisixResult, serviceResult, strength)
	return map[string]any{
		"input": map[string]any{
			"host":            req.Host,
			"uri":             req.URI,
			"status":          req.Status,
			"range_start":     start.Format(time.RFC3339Nano),
			"range_end":       end.Format(time.RFC3339Nano),
			"range_mode":      mode,
			"include_options": req.IncludeOptions,
			"skip_apisix":     skipAPISIX,
			"limit":           limit,
		},
		"route":              routeOutput(route, serviceIndex, serviceURIField),
		"investigation_mode": investigationMode(apisixResult, serviceResult),
		"evidence_strength":  strength,
		"gaps":               gaps,
		"findings":           findings,
		"apisix":             apisixResult,
		"service_logs":       serviceResult,
	}, nil
}

func (c *Client) matchRoute(host, uri string) *CorrelationRoute {
	for _, route := range c.correlation.Routes {
		if !strings.EqualFold(route.Host, host) {
			continue
		}
		if route.PathPrefix == "" || uri == "" || strings.HasPrefix(uri, route.PathPrefix) {
			matched := route
			return &matched
		}
	}
	return nil
}

func (c *Client) searchAPISIX(ctx context.Context, config CorrelationConfig, index string, req CorrelateRequest, start, end time.Time, limit int) (map[string]any, error) {
	filters := []any{
		matchPhrase(config.APISIXHostField, req.Host),
		rangeFilter(config.TimeField, start, end),
	}
	if strings.TrimSpace(req.URI) != "" {
		filters = append(filters, matchPhrase(config.APISIXURIField, req.URI))
	}
	if strings.TrimSpace(req.Status) != "" {
		filters = append(filters, matchPhrase(config.APISIXStatusField, req.Status))
	}
	boolQuery := map[string]any{"filter": filters}
	if !req.IncludeOptions {
		boolQuery["must_not"] = []any{
			map[string]any{"match_phrase": map[string]any{config.APISIXReqField: "OPTIONS "}},
		}
	}
	body := map[string]any{
		"size":             limit,
		"timeout":          searchTimeout,
		"track_total_hits": false,
		"_source": []string{
			"@timestamp",
			"@timestamp_02",
			"host_02",
			"uri",
			"request",
			"status",
			"request_time",
			"upstream_response_time",
			"upstream_addr",
			"remote_addr",
			"http_x_forwarded_for",
			"http_user_agent",
			"trace_id",
			"traceId",
			"request_id",
			"requestId",
			"x_request_id",
		},
		"sort": []any{map[string]any{
			config.TimeField: map[string]any{"order": "desc"},
		}},
		"query": map[string]any{"bool": boolQuery},
		"aggs": map[string]any{
			"request_time_stats": map[string]any{
				"stats": map[string]any{"field": "request_time"},
			},
			"request_time_percentiles": map[string]any{
				"percentiles": map[string]any{
					"field":    "request_time",
					"percents": []float64{50, 90, 95, 99},
				},
			},
			"upstream_response_time_stats": map[string]any{
				"stats": map[string]any{"field": "upstream_response_time"},
			},
		},
	}
	payload, err := c.postJSON(ctx, "/"+index+"/_search", body)
	if err != nil {
		return nil, err
	}
	hits := mapValue(payload["hits"])
	items := []map[string]any{}
	for _, raw := range anySlice(hits["hits"]) {
		hit := mapValue(raw)
		source := mapValue(hit["_source"])
		items = append(items, apisixItem(hit, source))
	}
	total := totalHits(hits)
	status := "available"
	if total == 0 {
		status = "empty"
	}
	return map[string]any{
		"status":         status,
		"index":          index,
		"total":          total,
		"item_count":     len(items),
		"items":          items,
		"latency":        latencySummary(mapValue(payload["aggregations"])),
		"result_order":   "timestamp_desc",
		"options_filter": !req.IncludeOptions,
	}, nil
}

func (c *Client) searchServiceLogs(ctx context.Context, config CorrelationConfig, route *CorrelationRoute, index, serviceURIField, uri, status string, start, end time.Time, limit int) (map[string]any, error) {
	must := []any{}
	query, exact := serviceLogQuery(route, serviceURIField, uri)
	if query != "" {
		must = append(must, map[string]any{
			"query_string": map[string]any{
				"query":            query,
				"analyze_wildcard": true,
			},
		})
	}
	if strings.TrimSpace(status) != "" {
		must = append(must, map[string]any{
			"query_string": map[string]any{
				"query":            "status:" + quoteQuery(status) + " OR " + quoteQuery(status),
				"analyze_wildcard": true,
			},
		})
	}
	body := map[string]any{
		"size":             limit,
		"timeout":          searchTimeout,
		"track_total_hits": false,
		"_source": []string{
			"@timestamp",
			"time",
			"level",
			"caller",
			"msg",
			"message",
			"evtName",
			"timeCost",
			"traceId",
			"trace_id",
			"userID",
			"user_id",
			"host",
			"host.name",
		},
		"sort": []any{map[string]any{
			config.TimeField: map[string]any{"order": "desc"},
		}},
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []any{rangeFilter(config.TimeField, start, end)},
				"must":   must,
			},
		},
	}
	payload, err := c.postJSON(ctx, "/"+index+"/_search", body)
	if err != nil {
		return nil, err
	}
	hits := mapValue(payload["hits"])
	items := []map[string]any{}
	for _, raw := range anySlice(hits["hits"]) {
		hit := mapValue(raw)
		source := mapValue(hit["_source"])
		items = append(items, serviceItem(hit, source))
	}
	total := totalHits(hits)
	resultStatus := "available"
	if total == 0 {
		resultStatus = "empty"
	}
	return map[string]any{
		"status":       resultStatus,
		"index":        index,
		"query":        query,
		"query_mode":   exact,
		"total":        total,
		"item_count":   len(items),
		"items":        items,
		"result_order": "timestamp_desc",
	}, nil
}

func serviceLogQuery(route *CorrelationRoute, serviceURIField, uri string) (string, string) {
	uri = strings.TrimSpace(uri)
	candidates := []string{}
	mode := "uri"
	if route != nil && route.ServiceEventField != "" && route.ServiceEventTemplate != "" {
		event := renderTemplate(route.ServiceEventTemplate, uri)
		if event != "" {
			candidates = append(candidates, fieldQuery(route.ServiceEventField, event))
			mode = "business_id"
		}
	}
	if serviceURIField != "" {
		candidates = append(candidates, fieldQuery(serviceURIField, uri))
	}
	if uri != "" {
		candidates = append(candidates, fieldQuery("message", uri))
	}
	if route != nil && route.ServiceFallbackQuery != "" {
		candidates = append(candidates, "("+route.ServiceFallbackQuery+")")
	}
	return strings.Join(dedupeStrings(candidates), " OR "), mode
}

func apisixItem(hit, source map[string]any) map[string]any {
	return compactMap(map[string]any{
		"timestamp":              sourceField(source, "@timestamp"),
		"timestamp_02":           sourceField(source, "@timestamp_02"),
		"host":                   sourceField(source, "host_02"),
		"uri":                    sourceField(source, "uri"),
		"request":                sourceField(source, "request"),
		"method":                 requestMethod(fmt.Sprint(sourceField(source, "request"))),
		"status":                 sourceField(source, "status"),
		"request_time":           sourceField(source, "request_time"),
		"upstream_response_time": sourceField(source, "upstream_response_time"),
		"upstream_addr":          sourceField(source, "upstream_addr"),
		"remote_addr":            sourceField(source, "remote_addr"),
		"http_x_forwarded_for":   sourceField(source, "http_x_forwarded_for"),
		"http_user_agent":        sourceField(source, "http_user_agent"),
		"trace_id":               firstValue(source, "trace_id", "traceId"),
		"request_id":             firstValue(source, "request_id", "requestId", "x_request_id"),
		"index":                  hit["_index"],
	})
}

func serviceItem(hit, source map[string]any) map[string]any {
	return compactMap(map[string]any{
		"timestamp": sourceField(source, "@timestamp"),
		"time":      sourceField(source, "time"),
		"level":     sourceField(source, "level"),
		"caller":    sourceField(source, "caller"),
		"msg":       sourceField(source, "msg"),
		"message":   sourceField(source, "message"),
		"evtName":   sourceField(source, "evtName"),
		"timeCost":  sourceField(source, "timeCost"),
		"traceId":   firstValue(source, "traceId", "trace_id"),
		"userID":    firstValue(source, "userID", "user_id"),
		"host":      sourceField(source, "host.name"),
		"index":     hit["_index"],
	})
}

func correlationRange(req CorrelateRequest) (time.Time, time.Time, string, error) {
	window := req.WindowSeconds
	if window <= 0 {
		window = 300
	}
	if window > maxCorrelationWindowSeconds {
		window = maxCorrelationWindowSeconds
	}
	if req.At != "" {
		at, err := parseAt(req.At)
		if err != nil {
			return time.Time{}, time.Time{}, "", err
		}
		return at.Add(-time.Duration(window) * time.Second), at.Add(time.Duration(window) * time.Second), "around_at", nil
	}
	since := req.SinceSeconds
	if since <= 0 {
		since = 900
	}
	if since > maxCorrelationSinceSeconds {
		since = maxCorrelationSinceSeconds
	}
	end := time.Now()
	return end.Add(-time.Duration(since) * time.Second), end, "since_now", nil
}

func parseAt(raw string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if strings.Contains(layout, "Z07") {
			if t, err := time.Parse(layout, raw); err == nil {
				return t, nil
			}
			continue
		}
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("at must be RFC3339 or yyyy-mm-dd HH:MM[:SS]")
}

func rangeFilter(field string, start, end time.Time) map[string]any {
	return map[string]any{
		"range": map[string]any{
			field: map[string]any{
				"gte": start.Format(time.RFC3339Nano),
				"lte": end.Format(time.RFC3339Nano),
			},
		},
	}
}

func routeOutput(route *CorrelationRoute, serviceIndex, serviceURIField string) map[string]any {
	out := map[string]any{
		"service_index":     serviceIndex,
		"service_uri_field": serviceURIField,
		"matched":           route != nil,
	}
	if route != nil {
		out["name"] = route.Name
		out["host"] = route.Host
		out["path_prefix"] = route.PathPrefix
		out["service"] = route.Service
		out["service_event_field"] = route.ServiceEventField
		out["service_event_template"] = route.ServiceEventTemplate
	}
	return compactMap(out)
}

func correlationStrength(apisix, service map[string]any) string {
	apisixAvailable := statusString(apisix) == "available" && totalInt(apisix) > 0
	serviceAvailable := statusString(service) == "available" && totalInt(service) > 0
	if !apisixAvailable && !serviceAvailable {
		return "missing"
	}
	if !apisixAvailable || !serviceAvailable {
		return "weak"
	}
	if sharedCorrelationID(apisix, service) {
		return "strong"
	}
	return "medium"
}

func investigationMode(apisix, service map[string]any) string {
	apisixAvailable := statusString(apisix) == "available" && totalInt(apisix) > 0
	serviceAvailable := statusString(service) == "available" && totalInt(service) > 0
	switch {
	case apisixAvailable && serviceAvailable:
		return "gateway_and_service"
	case serviceAvailable:
		return "service_only"
	case apisixAvailable:
		return "gateway_only"
	default:
		return "no_evidence"
	}
}

func correlationGaps(apisix, service map[string]any) []string {
	gaps := []string{}
	switch statusString(apisix) {
	case "skipped":
		gaps = append(gaps, "apisix_log_skipped")
	case "error":
		gaps = append(gaps, "apisix_log_query_error")
	case "empty":
		gaps = append(gaps, "apisix_log_empty")
	}
	if totalInt(apisix) > 0 && !itemsHaveAny(apisix, "trace_id", "request_id") {
		gaps = append(gaps, "apisix_trace_or_request_id_missing")
	}
	switch statusString(service) {
	case "skipped":
		gaps = append(gaps, "service_log_route_or_index_missing")
	case "empty":
		gaps = append(gaps, "service_log_empty_in_time_window")
	case "error":
		gaps = append(gaps, "service_log_query_error")
	}
	if itemsHaveAny(service, "traceId") && !itemsHaveAny(apisix, "trace_id", "request_id") {
		gaps = append(gaps, "trace_id_only_in_service_logs")
	}
	return gaps
}

func correlationFindings(apisix, service map[string]any, strength string) []string {
	findings := []string{
		"correlation_strength=" + strength,
		"investigation_mode=" + investigationMode(apisix, service),
	}
	if total := totalInt(apisix); total > 0 {
		findings = append(findings, "apisix_hits="+strconv.Itoa(total))
	}
	if total := totalInt(service); total > 0 {
		findings = append(findings, "service_log_hits="+strconv.Itoa(total))
	}
	if latency := mapValue(apisix["latency"]); len(latency) > 0 {
		if max := numberAt(latency, "request_time", "max"); !math.IsNaN(max) {
			findings = append(findings, fmt.Sprintf("apisix_request_time_max=%.3fs", max))
		}
		if p95 := numberAt(latency, "request_time_percentiles", "95.0"); !math.IsNaN(p95) {
			findings = append(findings, fmt.Sprintf("apisix_request_time_p95=%.3fs", p95))
		}
	}
	return findings
}

func latencySummary(aggs map[string]any) map[string]any {
	out := map[string]any{}
	if stats := compactMap(mapValue(aggs["request_time_stats"])); len(stats) > 0 {
		out["request_time"] = stats
	}
	if percentiles := mapValue(mapValue(aggs["request_time_percentiles"])["values"]); len(percentiles) > 0 {
		out["request_time_percentiles"] = percentiles
	}
	if stats := compactMap(mapValue(aggs["upstream_response_time_stats"])); len(stats) > 0 {
		out["upstream_response_time"] = stats
	}
	return out
}

func sourceField(source map[string]any, field string) any {
	if value, ok := source[field]; ok {
		return value
	}
	if strings.Contains(field, ".") {
		parts := strings.Split(field, ".")
		current := source
		for i, part := range parts {
			value, ok := current[part]
			if !ok {
				break
			}
			if i == len(parts)-1 {
				return value
			}
			next, ok := value.(map[string]any)
			if !ok {
				break
			}
			current = next
		}
	}
	message := source["message"]
	messageMap := map[string]any{}
	if raw, ok := message.(string); ok && strings.HasPrefix(strings.TrimSpace(raw), "{") {
		_ = json.Unmarshal([]byte(raw), &messageMap)
	}
	if value, ok := messageMap[field]; ok {
		return value
	}
	return nil
}

func firstValue(source map[string]any, fields ...string) any {
	for _, field := range fields {
		if value := sourceField(source, field); value != nil && fmt.Sprint(value) != "" {
			return value
		}
	}
	return nil
}

func fieldQuery(field, value string) string {
	value = strings.TrimSpace(value)
	if field == "" || value == "" {
		return ""
	}
	return field + ":" + quoteQuery(value)
}

func quoteQuery(value string) string {
	return strconv.Quote(strings.ReplaceAll(value, `\`, `\\`))
}

func renderTemplate(template, uri string) string {
	id := lastPathSegment(uri)
	out := strings.ReplaceAll(template, "${id}", id)
	out = strings.ReplaceAll(out, "${last_path_segment}", id)
	out = strings.ReplaceAll(out, "${uri}", uri)
	return strings.TrimSpace(out)
}

func lastPathSegment(raw string) string {
	if parsed, err := url.Parse(raw); err == nil && parsed.Path != "" {
		raw = parsed.Path
	}
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "/")
	return parts[len(parts)-1]
}

func requestMethod(request string) string {
	request = strings.TrimSpace(request)
	if request == "" {
		return ""
	}
	fields := strings.Fields(request)
	if len(fields) == 0 {
		return ""
	}
	upper := strings.ToUpper(fields[0])
	if regexp.MustCompile(`^[A-Z]+$`).MatchString(upper) {
		return upper
	}
	return ""
}

func totalHits(hits map[string]any) int {
	total := hits["total"]
	if mapped := mapValue(total); len(mapped) > 0 {
		return intFromAny(mapped["value"])
	}
	return intFromAny(total)
}

func totalInt(section map[string]any) int {
	return intFromAny(section["total"])
}

func statusString(section map[string]any) string {
	if status, ok := section["status"].(string); ok {
		return status
	}
	return ""
}

func itemsHaveAny(section map[string]any, keys ...string) bool {
	for _, raw := range anySlice(section["items"]) {
		item := mapValue(raw)
		for _, key := range keys {
			if value := item[key]; value != nil && fmt.Sprint(value) != "" {
				return true
			}
		}
	}
	return false
}

func sharedCorrelationID(apisix, service map[string]any) bool {
	ids := map[string]bool{}
	for _, raw := range anySlice(apisix["items"]) {
		item := mapValue(raw)
		for _, key := range []string{"trace_id", "request_id"} {
			if value := strings.TrimSpace(fmt.Sprint(item[key])); value != "" && value != "<nil>" {
				ids[value] = true
			}
		}
	}
	for _, raw := range anySlice(service["items"]) {
		item := mapValue(raw)
		for _, key := range []string{"traceId", "trace_id", "request_id"} {
			if value := strings.TrimSpace(fmt.Sprint(item[key])); value != "" && ids[value] {
				return true
			}
		}
	}
	return false
}

func compactMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range in {
		if value == nil {
			continue
		}
		if str, ok := value.(string); ok && str == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultCorrelationLimit
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		out, _ := strconv.Atoi(string(typed))
		return out
	default:
		out, _ := strconv.Atoi(fmt.Sprint(value))
		return out
	}
}

func numberAt(root map[string]any, keys ...string) float64 {
	var current any = root
	for _, key := range keys {
		mapped := mapValue(current)
		if mapped == nil {
			return math.NaN()
		}
		current = mapped[key]
	}
	return floatFromAny(current)
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		out, err := strconv.ParseFloat(fmt.Sprint(value), 64)
		if err != nil {
			return math.NaN()
		}
		return out
	}
}
