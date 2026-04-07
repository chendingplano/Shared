// nets-server runs an embedded NATS server process for local/service usage.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/spf13/cobra"
)

type serverConfig struct {
	Host      string
	Port      int
	HTTPPort  int
	StoreDir  string
	JetStream bool
	Username  string
	Password  string
	Token     string
}

func envOrDefault(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
}

func envIntOrDefault(key string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}
	return n
}

func envBoolOrDefault(key string, defaultValue bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return defaultValue
	}
	switch raw {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		return defaultValue
	}
}

func defaultConfigFromEnv() serverConfig {
	return serverConfig{
		Host:      envOrDefault("NATS_HOST", "127.0.0.1"),
		Port:      envIntOrDefault("NATS_PORT", 4222),
		HTTPPort:  envIntOrDefault("NATS_HTTP_PORT", 8222),
		StoreDir:  envOrDefault("NATS_STORE_DIR", "nats-data"),
		JetStream: envBoolOrDefault("NATS_JETSTREAM", true),
		Username:  envOrDefault("NATS_USER", ""),
		Password:  envOrDefault("NATS_PASS", ""),
		Token:     envOrDefault("NATS_TOKEN", ""),
	}
}

func optionsFromConfig(cfg serverConfig) (*server.Options, error) {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid NATS port: %d", cfg.Port)
	}
	if cfg.HTTPPort < 0 || cfg.HTTPPort > 65535 {
		return nil, fmt.Errorf("invalid NATS http monitoring port: %d", cfg.HTTPPort)
	}
	if cfg.Token != "" && (cfg.Username != "" || cfg.Password != "") {
		return nil, errors.New("set either NATS_TOKEN or NATS_USER/NATS_PASS, not both")
	}
	if (cfg.Username == "") != (cfg.Password == "") {
		return nil, errors.New("NATS_USER and NATS_PASS must be set together")
	}

	opts := &server.Options{
		ServerName:    "nets-server",
		Host:          cfg.Host,
		Port:          cfg.Port,
		HTTPPort:      cfg.HTTPPort,
		JetStream:     cfg.JetStream,
		StoreDir:      cfg.StoreDir,
		Username:      cfg.Username,
		Password:      cfg.Password,
		Authorization: cfg.Token,
	}
	return opts, nil
}

func createLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

func newRootCmd() *cobra.Command {
	cfg := defaultConfigFromEnv()
	logger := createLogger()

	cmd := &cobra.Command{
		Use:   "nets-server",
		Short: "Run a NATS server with JetStream support",
		Long: `Starts a NATS server process with optional JetStream persistence.

Environment variables:
  NATS_HOST         NATS listen host (default: 127.0.0.1)
  NATS_PORT         NATS client port (default: 4222)
  NATS_HTTP_PORT    NATS monitoring port, 0 disables (default: 8222)
  NATS_STORE_DIR    JetStream store directory (default: nats-data)
  NATS_JETSTREAM    Enable JetStream true/false (default: true)
  NATS_USER         Username for auth (optional, with NATS_PASS)
  NATS_PASS         Password for auth (optional, with NATS_USER)
  NATS_TOKEN        Token auth (optional; mutually exclusive with user/pass)
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := optionsFromConfig(cfg)
			if err != nil {
				return err
			}

			ns, err := server.NewServer(opts)
			if err != nil {
				return fmt.Errorf("failed to create nats server: %w", err)
			}

			ns.ConfigureLogger()
			go ns.Start()

			if !ns.ReadyForConnections(10 * time.Second) {
				return errors.New("nats server failed to become ready within 10s")
			}

			logger.Info("NATS server started",
				"host", opts.Host,
				"port", opts.Port,
				"http_port", opts.HTTPPort,
				"jetstream", opts.JetStream,
				"store_dir", opts.StoreDir,
			)

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			sig := <-sigCh
			logger.Info("shutdown signal received", "signal", sig.String())

			ns.Shutdown()
			ns.WaitForShutdown()
			logger.Info("NATS server stopped")
			return nil
		},
	}

	cmd.Flags().StringVar(&cfg.Host, "host", cfg.Host, "NATS listen host")
	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "NATS client port")
	cmd.Flags().IntVar(&cfg.HTTPPort, "http-port", cfg.HTTPPort, "NATS monitoring port (0 disables)")
	cmd.Flags().StringVar(&cfg.StoreDir, "store-dir", cfg.StoreDir, "JetStream storage directory")
	cmd.Flags().BoolVar(&cfg.JetStream, "jetstream", cfg.JetStream, "Enable JetStream")
	cmd.Flags().StringVar(&cfg.Username, "user", cfg.Username, "NATS username (use with --pass)")
	cmd.Flags().StringVar(&cfg.Password, "pass", cfg.Password, "NATS password (use with --user)")
	cmd.Flags().StringVar(&cfg.Token, "token", cfg.Token, "NATS token auth")

	return cmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
