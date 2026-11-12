package otel

import (
	"context"
	"testing"
)

func TestInitDisabledIsNoop(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{Enabled: false})
	if err != nil {
		t.Fatalf("Init disabled: %v", err)
	}
	if shutdown == nil {
		t.Fatalf("Init disabled returned nil shutdown")
	}
	shutdown()
}

func TestInitRuntimeDisabledHasNoRecorder(t *testing.T) {
	runtime, err := InitRuntime(context.Background(), Config{Enabled: false})
	if err != nil {
		t.Fatalf("InitRuntime disabled: %v", err)
	}
	if runtime.Shutdown == nil {
		t.Fatalf("InitRuntime disabled returned nil shutdown")
	}
	if runtime.Recorder != nil {
		t.Fatalf("disabled recorder = %+v, want nil", runtime.Recorder)
	}
	runtime.Shutdown()
}

func TestInitRuntimeMetricsImplyEnabled(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	runtime, err := InitRuntime(context.Background(), Config{
		MetricsEnabled: true,
		ServiceName:    "loom-test",
		ServiceVersion: "test",
		Endpoint:       "127.0.0.1:4318",
	})
	if err != nil {
		t.Fatalf("InitRuntime metrics implied enabled: %v", err)
	}
	if runtime.Recorder == nil {
		t.Fatalf("InitRuntime metrics implied enabled returned nil recorder")
	}
	runtime.Shutdown()
}

func TestInitEnabledWithLocalEndpoint(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	runtime, err := InitRuntime(context.Background(), Config{
		Enabled:        true,
		MetricsEnabled: true,
		ServiceName:    "loom-test",
		ServiceVersion: "test",
		Endpoint:       "127.0.0.1:4318",
	})
	if err != nil {
		t.Fatalf("Init enabled: %v", err)
	}
	if runtime.Recorder == nil {
		t.Fatalf("InitRuntime metrics returned nil recorder")
	}
	runtime.Shutdown()
}
