package prometheus

import "testing"

func TestParseDataSources(t *testing.T) {
	sources := ParseDataSources("", "node200=http://prometheus:9090,vm=http://vm-prometheus:9090")
	if len(sources) != 2 {
		t.Fatalf("len = %d", len(sources))
	}
	if sources[0].Name != "node200" || sources[0].URL != "http://prometheus:9090" {
		t.Fatalf("first = %#v", sources[0])
	}
	if sources[1].Name != "vm" || sources[1].URL != "http://vm-prometheus:9090" {
		t.Fatalf("second = %#v", sources[1])
	}
}

func TestNewRegistryKeepsSingleURLCompatibility(t *testing.T) {
	registry := NewRegistry("", "http://prometheus:9090", "")
	if !registry.Configured() {
		t.Fatal("expected configured registry")
	}
	if registry.DefaultSource() != "default" {
		t.Fatalf("default source = %s", registry.DefaultSource())
	}
	if _, source, err := registry.Client(""); err != nil || source != "default" {
		t.Fatalf("client source = %s err = %v", source, err)
	}
}

func TestNewRegistryDefaultSourceFallback(t *testing.T) {
	registry := NewRegistry("missing", "", "node200=http://prometheus:9090")
	if registry.DefaultSource() != "node200" {
		t.Fatalf("default source = %s", registry.DefaultSource())
	}
}
