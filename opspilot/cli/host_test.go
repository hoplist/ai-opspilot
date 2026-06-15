package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunHostDiskHuman(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/host/disk" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("host"); got != "node206" {
			t.Fatalf("host = %s", got)
		}
		_, _ = w.Write([]byte(`{
			"ok": true,
			"data": {
				"host": "node206",
				"filesystems": [{"path":"/var/lib/docker","mountpoint":"/","fstype":"xfs","avail_bytes":1073741824,"total_bytes":2147483648,"used_percent":50}],
				"top_paths": [{"path":"/var/lib/docker/containers","size_bytes":536870912,"depth":1}],
				"docker": {"available":true,"approx_reclaimable_bytes":268435456,"images_size_bytes":536870912},
				"container_logs": [{"container":"gitlab","log_driver":"json-file","size_bytes":536870912,"log_path":"/var/lib/docker/containers/id/id-json.log"}],
				"cleanup_plan": [{"risk":"controlled_mutation","summary":"Docker json-file log rotation is not configured.","evidence":"gitlab","recommendation":"Configure log rotation.","min_validation":"Run host disk again.","execution_boundary":"plan_only"}]
			},
			"warnings": []
		}`))
	}))
	defer server.Close()

	var out strings.Builder
	err := run([]string{"--backend-url", server.URL, "--output", "human", "host", "disk", "--host", "node206"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"Host disk: host=node206", "Filesystems:", "Top paths:", "Container logs:", "Cleanup plan:", "Docker json-file log rotation"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in output:\n%s", want, text)
		}
	}
}

func TestRunHostNetworkHuman(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/host/network" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("duration"); got != "3" {
			t.Fatalf("duration = %s", got)
		}
		_, _ = w.Write([]byte(`{
			"ok": true,
			"data": {
				"host": "node206",
				"duration_seconds": 3,
				"interfaces": [{"name":"eth0","rx_bps":1024,"tx_bps":2048,"rx_bytes":1048576,"tx_bytes":2097152}],
				"containers": [{"container":"gitlab","rx_bps":512,"tx_bps":256,"rx_bytes":1024,"tx_bytes":2048}],
				"tcp_states": {"ESTABLISHED": 2}
			},
			"warnings": []
		}`))
	}))
	defer server.Close()

	var out strings.Builder
	err := run([]string{"--backend-url", server.URL, "--output", "human", "host", "network", "--host", "node206", "--duration", "3"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"Host network: host=node206", "Interfaces:", "Containers:", "TCP states:", "eth0", "gitlab"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in output:\n%s", want, text)
		}
	}
}
