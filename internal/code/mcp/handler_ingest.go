package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nanjj/clog"
)

// ─── Argument structs ──────────────────────────────────────────────────────────

type ingestTraceSpan struct {
	TraceID      string `json:"traceId"`
	SpanID       string `json:"spanId"`
	ParentSpanID string `json:"parentSpanId,omitempty"`
	Service      string `json:"service"`
	Operation    string `json:"operation"`
	StartTime    int64  `json:"startTime"`
	Duration     int64  `json:"duration"`
	Status       string `json:"status,omitempty"`
	Tags         string `json:"tags,omitempty"`
}

type ingestTracesArgs struct {
	Project string            `json:"project"`
	Traces  []ingestTraceSpan `json:"traces"`
}

// ─── ingest_traces ──────────────────────────────────────────────────────────────

func registerIngestTraces(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ingest_traces",
		Description: "Ingest runtime traces to enhance the knowledge graph with execution paths, call frequencies, and latency data.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ingestTracesArgs) (*mcp.CallToolResult, any, error) {
		return s.handleIngestTraces(ctx, args), nil, nil
	})
}

func (s *Server) handleIngestTraces(ctx context.Context, args ingestTracesArgs) *mcp.CallToolResult {
	span, ctx := clog.StartSpanFromContext(ctx, "ingest_traces")
	defer span.Finish()
	clog.Info(ctx, "ingest_traces", "project", args.Project, "traceCount", len(args.Traces))

	info, err := s.resolveProject(args.Project)
	if err != nil {
		clog.Error(ctx, "error", "error", err.Error())
		return errorResult(err)
	}
	if len(args.Traces) == 0 {
		return errorResult(fmt.Errorf("traces is required"))
	}

	// Ingest traces using raw SQL via the store's underlying DB connection.
	db := s.store.DB()
	inserted := 0
	for _, t := range args.Traces {
		result, err := db.Exec(`
			INSERT INTO traces (project_id, trace_id, span_id, parent_span_id, service, operation, start_time, duration, status, tags)
			VALUES ((SELECT id FROM projects WHERE name = ?), ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, info.Name, t.TraceID, t.SpanID, t.ParentSpanID, t.Service, t.Operation,
			t.StartTime, t.Duration, t.Status, t.Tags)
		if err != nil {
			continue
		}
		n, _ := result.RowsAffected()
		if n > 0 {
			inserted++
		}
	}

	clog.Info(ctx, "ingest_traces_result", "inserted", inserted, "total", len(args.Traces))
	return jsonResult(map[string]any{
		"inserted": inserted,
		"total":    len(args.Traces),
	})
}
