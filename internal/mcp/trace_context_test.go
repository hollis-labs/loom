package mcp

import (
	"context"
	"testing"

	gmcp "github.com/hollis-labs/libs/plugin-mcp/go-mcp/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceToolMetadataPreservesRequestContext(t *testing.T) {
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })

	type requestKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), requestKey{}, "request"))
	cancel()
	ctx = gmcp.WithMeta(ctx, map[string]any{
		"_traceparent": "00-11111111111111111111111111111111-2222222222222222-01",
	})
	called := false
	handler := traceTool("test", func(ctx context.Context, _ map[string]any) (any, error) {
		called = true
		if ctx.Err() != context.Canceled || ctx.Value(requestKey{}) != "request" {
			t.Fatal("trace extraction lost request cancellation or values")
		}
		if got := trace.SpanContextFromContext(ctx).TraceID().String(); got != "11111111111111111111111111111111" {
			t.Fatalf("trace ID = %s, want protocol metadata trace", got)
		}
		return nil, nil
	}, nil)
	if _, err := handler(ctx, map[string]any{
		"_traceparent": "00-33333333333333333333333333333333-4444444444444444-01",
	}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("handler not reached")
	}
}
