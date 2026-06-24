package mcp

import (
	"context"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/v3/assert"
)

// clearTraceEnv unsets CLOG_TRACEPARENT if set and returns a restore function.
// Use at the start of tests that check env state to avoid pollution from the
// parent process or other tests.
func clearTraceEnv() func() {
	prev, wasSet := os.LookupEnv("CLOG_TRACEPARENT")
	os.Unsetenv("CLOG_TRACEPARENT")
	if wasSet {
		return func() { os.Setenv("CLOG_TRACEPARENT", prev) }
	}
	return func() {}
}

func TestTracingMiddleware_ExtractTraceparent(t *testing.T) {
	defer clearTraceEnv()()

	var capturedCtx context.Context
	var capturedEnv string

	handler := TracingMiddleware(func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		capturedCtx = ctx
		capturedEnv = os.Getenv("CLOG_TRACEPARENT")
		return &mcp.CallToolResult{}, nil
	})

	testTP := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"

	params := &mcp.CallToolParamsRaw{Name: "test_tool"}
	params.SetMeta(map[string]any{"traceparent": testTP})

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Params: params,
	}

	_, err := handler(context.Background(), "tools/call", req)
	assert.NilError(t, err)

	// Verify context carries the traceparent.
	assert.Equal(t, testTP, getTraceparent(capturedCtx))

	// Verify env was set during handler execution.
	assert.Equal(t, testTP, capturedEnv)

	// Verify env is restored after handler returns.
	assert.Equal(t, "", os.Getenv("CLOG_TRACEPARENT"))
}

func TestTracingMiddleware_SkipsNonCallMethod(t *testing.T) {
	defer clearTraceEnv()()

	var called bool
	handler := TracingMiddleware(func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		called = true
		return &mcp.CallToolResult{}, nil
	})

	testTP := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	params := &mcp.CallToolParamsRaw{Name: "test"}
	params.SetMeta(map[string]any{"traceparent": testTP})

	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: params}

	// "tools/list" should NOT trigger traceparent extraction.
	_, err := handler(context.Background(), "tools/list", req)
	assert.NilError(t, err)
	assert.Assert(t, called)

	// Env should NOT be set for non-call methods (neither during nor after).
	assert.Equal(t, "", os.Getenv("CLOG_TRACEPARENT"))
}

func TestTracingMiddleware_EmptyMeta(t *testing.T) {
	defer clearTraceEnv()()

	var capturedCtx context.Context
	handler := TracingMiddleware(func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		capturedCtx = ctx
		return &mcp.CallToolResult{}, nil
	})

	// No Meta set at all.
	params := &mcp.CallToolParamsRaw{Name: "test"}
	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: params}

	_, err := handler(context.Background(), "tools/call", req)
	assert.NilError(t, err)

	// Context should not have a traceparent.
	assert.Equal(t, "", getTraceparent(capturedCtx))
}

func TestTracingMiddleware_RestoresPrevEnv(t *testing.T) {
	// Set a prior CLOG_TRACEPARENT to simulate a parent process context.
	prevTP := "00-deadbeefdeadbeefdeadbeefdeadbeef-deadbeefdeadbeef-01"
	os.Setenv("CLOG_TRACEPARENT", prevTP)
	defer os.Unsetenv("CLOG_TRACEPARENT")

	var capturedEnv string
	handler := TracingMiddleware(func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		capturedEnv = os.Getenv("CLOG_TRACEPARENT")
		return &mcp.CallToolResult{}, nil
	})

	newTP := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	params := &mcp.CallToolParamsRaw{Name: "test"}
	params.SetMeta(map[string]any{"traceparent": newTP})
	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: params}

	_, err := handler(context.Background(), "tools/call", req)
	assert.NilError(t, err)

	// During handler, the overridden traceparent should be active.
	assert.Equal(t, newTP, capturedEnv)

	// After handler returns, the previous value should be restored.
	assert.Equal(t, prevTP, os.Getenv("CLOG_TRACEPARENT"))
}

func TestStartSpanWithMeta_ViaTraceparent(t *testing.T) {
	defer clearTraceEnv()()

	// With no global tracer set, StartSpanWithMeta falls through to
	// clog.StartSpanFromContext which returns a no-op span. We verify
	// the function doesn't panic and returns a valid span.
	testTP := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	ctx := withTraceparent(context.Background(), testTP)

	span, newCtx := StartSpanWithMeta(ctx, "test-op")
	assert.Assert(t, span != nil)
	assert.Assert(t, newCtx != nil)
	span.Finish()
}

// TestGetTraceparent verifies the get/with round-trip.
func TestGetTraceparent(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", getTraceparent(ctx))

	tp := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	ctx = withTraceparent(ctx, tp)
	assert.Equal(t, tp, getTraceparent(ctx))
}
