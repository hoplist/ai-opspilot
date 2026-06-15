package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateConfigRequiresTokenForNonLocalListenHost(t *testing.T) {
	if err := validateConfig(config{host: "0.0.0.0"}); err == nil {
		t.Fatal("expected non-local listener without token to fail")
	}
	if err := validateConfig(config{host: "192.168.48.206"}); err == nil {
		t.Fatal("expected concrete non-local listener without token to fail")
	}
}

func TestValidateConfigAllowsLocalOrTokenProtectedAgent(t *testing.T) {
	for _, cfg := range []config{
		{host: "127.0.0.1"},
		{host: "localhost"},
		{host: "::1"},
		{host: "0.0.0.0", token: "secret"},
	} {
		if err := validateConfig(cfg); err != nil {
			t.Fatalf("validateConfig(%+v) = %v", cfg, err)
		}
	}
}

func TestParseDiskPathsKeepsAbsoluteUniquePaths(t *testing.T) {
	paths := parseDiskPaths("/var/log, relative,/var/log,/opt/../opt,/data*")
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths, got %#v", paths)
	}
	if paths[0] != "/var/log" || paths[1] != "/opt" || paths[2] != "/data*" {
		t.Fatalf("unexpected paths: %#v", paths)
	}
}

func TestExpandHostPathPatternsIncludesDataMounts(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"data", "data00", "data01"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	paths := expandHostPathPatterns(root, []string{"/data*"})
	if len(paths) != 3 {
		t.Fatalf("paths = %#v", paths)
	}
	want := map[string]bool{"/data": true, "/data00": true, "/data01": true}
	for _, item := range paths {
		if !want[item] {
			t.Fatalf("unexpected path %s in %#v", item, paths)
		}
	}
}

func TestMapHostPathUsesHostRoot(t *testing.T) {
	root := filepath.Join("C:\\hostroot")
	got := mapHostPath(root, "/var/lib/docker")
	want := filepath.Join(root, "var", "lib", "docker")
	if got != want {
		t.Fatalf("mapped path = %q, want %q", got, want)
	}
}

func TestCollectTopPathsSkipsSymlink(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "var", "log")
	if err := os.MkdirAll(filepath.Join(logDir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "app", "app.log"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(logDir, "linked")); err != nil {
		t.Logf("symlink unavailable, skipping symlink assertion: %v", err)
	}
	items, warnings := collectTopPaths(context.Background(), root, []string{"/var/log"}, 2, 10)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	seenApp := false
	for _, item := range items {
		if item.Path == "/var/log/linked" {
			t.Fatalf("symlink should not be reported: %#v", items)
		}
		if item.Path == "/var/log/app" && item.SizeBytes == int64(len("hello")) {
			seenApp = true
		}
	}
	if !seenApp {
		t.Fatalf("expected app log usage in %#v", items)
	}
}
