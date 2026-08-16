package otel

import (
	"context"
	"os"
	"strings"
	"time"

	hotel "github.com/hollis-labs/go-otel"
	"go.opentelemetry.io/otel"
)

type Config struct {
	Enabled        bool
	MetricsEnabled bool
	ServiceName    string
	ServiceVersion string
	Environment    string
	Endpoint       string
}

type Runtime struct {
	Shutdown func()
	Recorder *hotel.Recorder
}

func EnabledFromEnv() bool {
	for _, key := range []string{"LOOM_OTEL_ENABLED", "OTEL_ENABLED"} {
		if enabled(os.Getenv(key)) {
			return true
		}
	}
	return false
}

func MetricsEnabledFromEnv() bool {
	for _, key := range []string{"LOOM_OTEL_METRICS_ENABLED", "OTEL_METRICS_ENABLED"} {
		if enabled(os.Getenv(key)) {
			return true
		}
	}
	return false
}

func Init(ctx context.Context, cfg Config) (func(), error) {
	runtime, err := InitRuntime(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return runtime.Shutdown, nil
}

func InitRuntime(ctx context.Context, cfg Config) (Runtime, error) {
	if cfg.MetricsEnabled {
		cfg.Enabled = true
	}
	if !cfg.Enabled {
		return Runtime{Shutdown: func() {}}, nil
	}
	serviceName := strings.TrimSpace(cfg.ServiceName)
	if serviceName == "" {
		serviceName = "loom"
	}
	serviceVersion := strings.TrimSpace(cfg.ServiceVersion)
	if serviceVersion == "" {
		serviceVersion = "unknown"
	}
	environment := strings.TrimSpace(cfg.Environment)
	if environment == "" {
		environment = "development"
	}
	opts := []hotel.Option{
		hotel.WithServiceName(serviceName),
		hotel.WithServiceVersion(serviceVersion),
		hotel.WithEnvironment(environment),
	}
	if cfg.MetricsEnabled {
		opts = append(opts, hotel.WithMetricsEnabled())
	}
	if endpoint := strings.TrimSpace(cfg.Endpoint); endpoint != "" {
		opts = append(opts, hotel.WithOTLPEndpoint(endpoint))
	}
	shutdown, err := hotel.Init(ctx, opts...)
	if err != nil {
		return Runtime{}, err
	}
	runtime := Runtime{Shutdown: func() {
		_ = hotel.ShutdownWithTimeout(shutdown, 5*time.Second)
	}}
	if cfg.MetricsEnabled {
		recorder, err := hotel.RegisterRecorder(otel.Meter(serviceName), serviceName)
		if err != nil {
			runtime.Shutdown()
			return Runtime{}, err
		}
		runtime.Recorder = recorder
	}
	return runtime, nil
}

func enabled(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
