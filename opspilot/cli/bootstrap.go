package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type bootstrapResult struct {
	SourceNamespace string   `json:"source_namespace"`
	SourceSecret    string   `json:"source_secret"`
	TargetSelector  string   `json:"target_selector"`
	SecretName      string   `json:"secret_name"`
	Synced          []string `json:"synced_namespaces"`
}

type kubeBootstrapClient struct {
	host      string
	port      string
	tokenPath string
	caPath    string
	http      *http.Client
}

func bootstrapCommand(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "namespace-secrets" {
		return fmt.Errorf("expected: bootstrap namespace-secrets")
	}
	fs := flag.NewFlagSet("bootstrap namespace-secrets", flag.ExitOnError)
	sourceNamespace := fs.String("source-namespace", env("SOURCE_NAMESPACE", "opspilot"), "source namespace")
	sourceSecret := fs.String("source-secret", env("SOURCE_SECRET", "gitlab-registry-pull"), "source pull secret")
	targetSelector := fs.String("target-selector", env("TARGET_SELECTOR", "opspilot.io/managed=true"), "managed namespace label selector")
	secretName := fs.String("secret-name", env("TARGET_SECRET", "gitlab-registry-pull"), "target pull secret name")
	_ = fs.Parse(args[1:])

	client, err := newKubeBootstrapClient()
	if err != nil {
		return err
	}
	result, err := client.syncRegistryPullSecret(context.Background(), *sourceNamespace, *sourceSecret, *targetSelector, *secretName)
	if err != nil {
		return err
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(body))
	return err
}

func newKubeBootstrapClient() (*kubeBootstrapClient, error) {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	if host == "" {
		return nil, fmt.Errorf("bootstrap namespace-secrets requires in-cluster Kubernetes service environment")
	}
	tokenPath := env("OPSPILOT_SERVICEACCOUNT_TOKEN", "/var/run/secrets/kubernetes.io/serviceaccount/token")
	if _, err := os.Stat(tokenPath); err != nil {
		return nil, err
	}
	return &kubeBootstrapClient{
		host:      host,
		port:      env("KUBERNETES_SERVICE_PORT", "443"),
		tokenPath: tokenPath,
		caPath:    env("OPSPILOT_SERVICEACCOUNT_CA", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"),
		http:      &http.Client{Timeout: 20 * time.Second},
	}, nil
}

func (c *kubeBootstrapClient) syncRegistryPullSecret(ctx context.Context, sourceNamespace, sourceSecret, selector, secretName string) (bootstrapResult, error) {
	source, err := c.getJSON(ctx, "/api/v1/namespaces/"+url.PathEscape(sourceNamespace)+"/secrets/"+url.PathEscape(sourceSecret))
	if err != nil {
		return bootstrapResult{}, err
	}
	data, _ := source["data"].(map[string]any)
	payload, _ := data[".dockerconfigjson"].(string)
	if payload == "" {
		return bootstrapResult{}, fmt.Errorf("source secret %s/%s is missing .dockerconfigjson", sourceNamespace, sourceSecret)
	}

	namespaces, err := c.listManagedNamespaces(ctx, selector)
	if err != nil {
		return bootstrapResult{}, err
	}
	result := bootstrapResult{
		SourceNamespace: sourceNamespace,
		SourceSecret:    sourceSecret,
		TargetSelector:  selector,
		SecretName:      secretName,
		Synced:          []string{},
	}
	for _, namespace := range namespaces {
		if err := c.applyPullSecret(ctx, namespace, secretName, sourceNamespace, sourceSecret, payload); err != nil {
			return result, err
		}
		result.Synced = append(result.Synced, namespace)
	}
	return result, nil
}

func (c *kubeBootstrapClient) listManagedNamespaces(ctx context.Context, selector string) ([]string, error) {
	path := "/api/v1/namespaces"
	if selector != "" {
		path += "?" + url.Values{"labelSelector": []string{selector}}.Encode()
	}
	payload, err := c.getJSON(ctx, path)
	if err != nil {
		return nil, err
	}
	rawItems, _ := payload["items"].([]any)
	names := []string{}
	for _, raw := range rawItems {
		item, _ := raw.(map[string]any)
		meta, _ := item["metadata"].(map[string]any)
		name, _ := meta["name"].(string)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func (c *kubeBootstrapClient) applyPullSecret(ctx context.Context, namespace, secretName, sourceNamespace, sourceSecret, dockerConfig string) error {
	body := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      secretName,
			"namespace": namespace,
			"labels": map[string]string{
				"app.kubernetes.io/name":    "opspilot-namespace-bootstrap",
				"app.kubernetes.io/part-of": "opspilot",
				"opspilot.io/managed":       "true",
			},
			"annotations": map[string]string{
				"opspilot.io/source-secret": sourceNamespace + "/" + sourceSecret,
			},
		},
		"type": "kubernetes.io/dockerconfigjson",
		"data": map[string]string{
			".dockerconfigjson": dockerConfig,
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	path := "/api/v1/namespaces/" + url.PathEscape(namespace) + "/secrets/" + url.PathEscape(secretName) + "?fieldManager=opspilot-namespace-bootstrap&force=true"
	return c.request(ctx, http.MethodPatch, path, bytes.NewReader(payload), "application/apply-patch+yaml", nil)
}

func (c *kubeBootstrapClient) getJSON(ctx context.Context, path string) (map[string]any, error) {
	var out map[string]any
	err := c.request(ctx, http.MethodGet, path, nil, "", &out)
	return out, err
}

func (c *kubeBootstrapClient) request(ctx context.Context, method, path string, body io.Reader, contentType string, out any) error {
	tokenBytes, err := os.ReadFile(c.tokenPath)
	if err != nil {
		return err
	}
	endpoint := "https://" + c.host + ":" + c.port + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tokenBytes)))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	client := c.http
	if fileExists(c.caPath) {
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		ca, err := os.ReadFile(c.caPath)
		if err == nil {
			pool.AppendCertsFromPEM(ca)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
		client = &http.Client{Timeout: 20 * time.Second, Transport: transport}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("kubernetes api %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return err
		}
	}
	return nil
}
