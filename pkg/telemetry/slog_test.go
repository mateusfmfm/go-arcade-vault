package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestContextHandler_AddsTraceIDs(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	t.Cleanup(func() { span.End() })

	var buf bytes.Buffer
	logger := slog.New(NewContextHandler(slog.NewJSONHandler(&buf, nil)))
	logger.InfoContext(ctx, "hello")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("parse log: %v", err)
	}

	sc := span.SpanContext()
	if rec["trace_id"] != sc.TraceID().String() {
		t.Fatalf("trace_id=%v want %s", rec["trace_id"], sc.TraceID())
	}
	if rec["span_id"] != sc.SpanID().String() {
		t.Fatalf("span_id=%v want %s", rec["span_id"], sc.SpanID())
	}
}

func TestContextHandler_OmitsIDsWithoutSpan(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewContextHandler(slog.NewJSONHandler(&buf, nil)))
	logger.InfoContext(context.Background(), "hello")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("parse log: %v", err)
	}
	if _, ok := rec["trace_id"]; ok {
		t.Fatal("expected no trace_id without a span")
	}
}
