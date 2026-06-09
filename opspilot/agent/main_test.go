package main

import "testing"

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
