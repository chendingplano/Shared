package main

import (
	"os"
	"testing"
)

func TestDefaultConfigFromEnv(t *testing.T) {
	t.Setenv("NATS_HOST", "0.0.0.0")
	t.Setenv("NATS_PORT", "5222")
	t.Setenv("NATS_HTTP_PORT", "8222")
	t.Setenv("NATS_STORE_DIR", "/tmp/nats-data")
	t.Setenv("NATS_JETSTREAM", "false")
	t.Setenv("NATS_USER", "alice")
	t.Setenv("NATS_PASS", "secret")
	t.Setenv("NATS_TOKEN", "")

	cfg := defaultConfigFromEnv()

	if cfg.Host != "0.0.0.0" {
		t.Fatalf("expected host from env, got %q", cfg.Host)
	}
	if cfg.Port != 5222 {
		t.Fatalf("expected port 5222, got %d", cfg.Port)
	}
	if cfg.HTTPPort != 8222 {
		t.Fatalf("expected http port 8222, got %d", cfg.HTTPPort)
	}
	if cfg.StoreDir != "/tmp/nats-data" {
		t.Fatalf("expected store dir from env, got %q", cfg.StoreDir)
	}
	if cfg.JetStream {
		t.Fatalf("expected jetstream disabled from env")
	}
	if cfg.Username != "alice" || cfg.Password != "secret" {
		t.Fatalf("expected user/pass from env")
	}
}

func TestOptionsFromConfig_RejectsTokenAndUserPass(t *testing.T) {
	cfg := serverConfig{
		Host:      "127.0.0.1",
		Port:      4222,
		HTTPPort:  8222,
		StoreDir:  "nats-data",
		JetStream: true,
		Username:  "alice",
		Password:  "secret",
		Token:     "token",
	}

	_, err := optionsFromConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error when token and user/pass are both set")
	}
}

func TestOptionsFromConfig_BuildsJetStreamOptions(t *testing.T) {
	cfg := serverConfig{
		Host:      "127.0.0.1",
		Port:      4222,
		HTTPPort:  8222,
		StoreDir:  "nats-data",
		JetStream: true,
	}

	opts, err := optionsFromConfig(cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !opts.JetStream {
		t.Fatalf("expected jetstream to be enabled")
	}
	if opts.StoreDir != "nats-data" {
		t.Fatalf("expected store dir to be nats-data, got %q", opts.StoreDir)
	}
	if opts.HTTPPort != 8222 {
		t.Fatalf("expected http port 8222, got %d", opts.HTTPPort)
	}
}

func TestParseEnvIntDefault(t *testing.T) {
	_ = os.Unsetenv("NATS_PORT")
	if got := envIntOrDefault("NATS_PORT", 4222); got != 4222 {
		t.Fatalf("expected default 4222, got %d", got)
	}

	t.Setenv("NATS_PORT", "invalid")
	if got := envIntOrDefault("NATS_PORT", 4222); got != 4222 {
		t.Fatalf("expected default on invalid int, got %d", got)
	}
}
