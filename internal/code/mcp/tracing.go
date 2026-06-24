package mcp

import (
	"context"
	"os"

	"github.com/nanjj/clog"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	opentracing "github.com/opentracing/opentracing-go"
)

// contextTraceparentKey holds a W3C traceparent string extracted from the
// incoming MCP request's _meta field. It is set by the receiving middleware
// and consumed by StartSpanWithMeta.
type contextTraceparentKey struct{}

// withTraceparent returns a context carrying the given traceparent string.
func withTraceparent(ctx context.Context, tp string) context.Context {
	return context.WithValue(ctx, contextTraceparentKey{}, tp)
}

// getTraceparent extracts a W3C traceparent string from ctx, or "".
func getTraceparent(ctx context.Context) string {
	v, _ := ctx.Value(contextTraceparentKey{}).(string)
	return v
}

// TracingMiddleware is MCP receiving middleware that extracts a W3C
// traceparent from the _meta field of incoming tool call requests and
// makes it available to handler spans via context.
//
// This enables end-to-end distributed tracing across process boundaries:
//
//	dscli (MCP client)
//	  └─ span for "search_graph"
//	       └─ injects traceparent into _meta
//	          └─ slingshot code serve (MCP server)
//	               └─ handler span as child of dscli's span
//
// The traceparent is carried both in the context (for thread safety) and
// in the process environment (for clog's ExtractFromEnv fallback). For
// stdio transport (single-threaded), the env-var approach is sufficient.
// For future concurrent transports, the context value provides a safe
// alternative.
func TracingMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method == "tools/call" {
			if meta := req.GetParams().GetMeta(); meta != nil {
				if tp, ok := meta["traceparent"].(string); ok && tp != "" {
					// Inject into context for thread-safe access.
					ctx = withTraceparent(ctx, tp)
					// Also inject into environment so clog.StartSpanFromContext's
					// ExtractFromEnv can find it. Save and restore to avoid
					// overwriting any process-level traceparent.
					prev, wasSet := os.LookupEnv("CLOG_TRACEPARENT")
					os.Setenv("CLOG_TRACEPARENT", tp)
					defer func() {
						if wasSet {
							os.Setenv("CLOG_TRACEPARENT", prev)
						} else {
							os.Unsetenv("CLOG_TRACEPARENT")
						}
					}()
				}
			}
		}
		return next(ctx, method, req)
	}
}

// StartSpanWithMeta starts a new span using the best available parent context.
//
// Priority order:
//  1. Parent span already in ctx (standard OpenTracing)
//  2. W3C traceparent from context (set by TracingMiddleware from _meta)
//  3. W3C traceparent from env (set by TracingMiddleware or parent process)
//  4. Root span (no parent)
//
// Use this in all MCP tool handlers instead of clog.StartSpanFromContext
// to enable cross-process trace propagation.
func StartSpanWithMeta(ctx context.Context, name string, opts ...opentracing.StartSpanOption) (opentracing.Span, context.Context) {
	// If ctx already has a parent span, use it directly (standard path).
	if parent := opentracing.SpanFromContext(ctx); parent != nil {
		return clog.StartSpanFromContext(ctx, name, opts...)
	}

	// Check context for traceparent injected by middleware.
	if tp := getTraceparent(ctx); tp != "" {
		// Temporarily set env var so clog.ExtractFromEnv picks it up.
		prev, wasSet := os.LookupEnv("CLOG_TRACEPARENT")
		os.Setenv("CLOG_TRACEPARENT", tp)
		span, newCtx := clog.StartSpanFromContext(ctx, name, opts...)
		if wasSet {
			os.Setenv("CLOG_TRACEPARENT", prev)
		} else {
			os.Unsetenv("CLOG_TRACEPARENT")
		}
		return span, newCtx
	}

	// Fallback: use env-based propagation (process-level or middleware-set).
	return clog.StartSpanFromContext(ctx, name, opts...)
}
