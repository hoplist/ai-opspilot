package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func fetchMetricItems(backendURL, query, source string) ([]metricItem, error) {
	body, err := get(backendURL, "/api/metrics/query", url.Values{"query": {query}, "source": {source}})
	if err != nil {
		return nil, err
	}
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	var data metricItemsData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, err
	}
	return data.Items, nil
}

func getJSONMap(backendURL, endpoint string, values url.Values) (map[string]any, error) {
	body, err := get(backendURL, endpoint, values)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func get(baseURL, endpoint string, values url.Values) ([]byte, error) {
	clean := url.Values{}
	for key, vals := range values {
		if len(vals) > 0 && vals[0] != "" {
			clean.Set(key, vals[0])
		}
	}
	target := strings.TrimRight(baseURL, "/") + endpoint
	if encoded := clean.Encode(); encoded != "" {
		target += "?" + encoded
	}
	ctx, cancel := context.WithTimeout(context.Background(), cliHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backend returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if json.Valid(body) {
		return body, nil
	}
	return nil, fmt.Errorf("backend returned non-json response")
}

func post(baseURL, endpoint string, values url.Values) ([]byte, error) {
	clean := url.Values{}
	for key, vals := range values {
		if len(vals) > 0 && vals[0] != "" {
			clean.Set(key, vals[0])
		}
	}
	target := strings.TrimRight(baseURL, "/") + endpoint
	ctx, cancel := context.WithTimeout(context.Background(), cliHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(clean.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backend returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if json.Valid(body) {
		return body, nil
	}
	return nil, fmt.Errorf("backend returned non-json response")
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}
