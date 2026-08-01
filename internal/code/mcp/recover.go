// Package mcp — panic recovery middleware.
package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/clog"
)

// RecoverMiddleware converts panics in handlers into error responses instead
// of crashing the server process. A crash mid-call manifests to the MCP
// client as a broken pipe / EOF with no diagnostics; recovering and returning
// a JSON-RPC error keeps the session alive and surfaces the failure.
//
// Register it LAST (outermost) so it wraps every other middleware and handler:
//
//	srv.AddReceivingMiddleware(codemcp.TracingMiddleware)
//	srv.AddReceivingMiddleware(codemcp.FixDoubleEncodedJSON)
//	srv.AddReceivingMiddleware(codemcp.RecoverMiddleware)
func RecoverMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (result mcp.Result, err error) {
		defer func() {
			if r := recover(); r != nil {
				clog.Error(ctx, "panic_recovered",
					"method", method,
					"panic", fmt.Sprint(r))
				err = fmt.Errorf("internal error (panic recovered in %s): %v", method, r)
			}
		}()
		return next(ctx, method, req)
	}
}
