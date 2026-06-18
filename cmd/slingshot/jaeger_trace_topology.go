package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdJaegerTraceTopology 实现 "slingshot jaeger trace topology <traceID>"。
type cmdJaegerTraceTopology struct {
	global     *cmdGlobal
	jaeger     *cmdJaeger
	trace      *cmdJaegerTrace
	jsonOutput bool
}

func (c *cmdJaegerTraceTopology) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "topology <traceID>"
	cmd.Short = i18n.G("Show trace service topology")
	cmd.Long = i18n.G(`Analyse a trace's spans to extract the service dependency graph:
which services communicated with which, their operations, span counts,
error counts, and average durations.`)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().BoolVar(&c.jsonOutput, "json", false, i18n.G("Output as JSON"))
	cmd.RunE = c.run
	return cmd
}

func (c *cmdJaegerTraceTopology) run(cmd *cobra.Command, args []string) error {
	data, err := c.jaeger.jaegerGet("/api/traces/" + args[0])
	if err != nil {
		return err
	}

	var resp jaegerTraceResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf(i18n.G("parsing trace response: %w"), err)
	}

	if len(resp.Data) == 0 {
		return fmt.Errorf(i18n.G("trace %s not found"), args[0])
	}

	trace := resp.Data[0]
	return printTopology(trace, c.jsonOutput)
}

// topologyService 记录一个服务在 trace 中的统计信息。
type topologyService struct {
	Name          string         `json:"name"`
	SpanCount     int            `json:"spanCount"`
	ErrorCount    int            `json:"errorCount"`
	TotalDuration int64          `json:"totalDurationUs"`
	Operations    map[string]int `json:"-"`
}

// topologyEdge 表示两个服务之间的依赖关系。
type topologyEdge struct {
	ParentService string `json:"parentService"`
	ChildService  string `json:"childService"`
	SpanCount     int    `json:"spanCount"`
}

func printTopology(trace jaegerTraceData, asJSON bool) error {
	// Index spans by ID for quick parent lookup.
	spanMap := make(map[string]jaegerSpan, len(trace.Spans))
	for _, span := range trace.Spans {
		spanMap[span.SpanID] = span
	}

	// Build service stats and dependency edges.
	services := make(map[string]*topologyService)
	edges := make(map[string]*topologyEdge) // key: "parentSvc|childSvc"

	for _, span := range trace.Spans {
		svcName := span.serviceName(trace.Processes)

		svc, ok := services[svcName]
		if !ok {
			svc = &topologyService{
				Name:       svcName,
				Operations: make(map[string]int),
			}
			services[svcName] = svc
		}
		svc.SpanCount++
		svc.Operations[span.OperationName]++
		svc.TotalDuration += span.Duration
		if span.hasError() {
			svc.ErrorCount++
		}

		// Each CHILD_OF reference creates a service-level dependency edge.
		for _, ref := range span.References {
			if ref.RefType != "CHILD_OF" {
				continue
			}
			parentSpan, ok := spanMap[ref.SpanID]
			if !ok {
				continue
			}
			parentSvc := parentSpan.serviceName(trace.Processes)
			if parentSvc == svcName {
				continue // skip self-loops
			}

			key := parentSvc + "|" + svcName
			edge, ok := edges[key]
			if !ok {
				edge = &topologyEdge{
					ParentService: parentSvc,
					ChildService:  svcName,
				}
				edges[key] = edge
			}
			edge.SpanCount++
		}
	}

	// Sort services by name for deterministic output.
	svcList := make([]*topologyService, 0, len(services))
	for _, svc := range services {
		svcList = append(svcList, svc)
	}
	sort.Slice(svcList, func(i, j int) bool {
		return svcList[i].Name < svcList[j].Name
	})

	if asJSON {
		return printTopologyJSON(svcList, edges)
	}
	return printTopologyText(svcList, edges)
}

func printTopologyJSON(svcs []*topologyService, edges map[string]*topologyEdge) error {
	// Prepare sorted edge list.
	edgeList := make([]*topologyEdge, 0, len(edges))
	for _, edge := range edges {
		edgeList = append(edgeList, edge)
	}
	sort.Slice(edgeList, func(i, j int) bool {
		a, b := edgeList[i], edgeList[j]
		if a.ParentService != b.ParentService {
			return a.ParentService < b.ParentService
		}
		return a.ChildService < b.ChildService
	})

	type jsonOutput struct {
		Services     []*topologyService `json:"services"`
		Dependencies []*topologyEdge    `json:"dependencies"`
	}
	out, err := json.MarshalIndent(jsonOutput{Services: svcs, Dependencies: edgeList}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func printTopologyText(svcs []*topologyService, edges map[string]*topologyEdge) error {
	fmt.Printf("Services (%d):\n", len(svcs))
	for _, svc := range svcs {
		// Format operations list, truncated.
		ops := make([]string, 0, len(svc.Operations))
		for op := range svc.Operations {
			ops = append(ops, op)
		}
		sort.Strings(ops)
		opStr := strings.Join(ops, ", ")
		if len(opStr) > 60 {
			opStr = opStr[:57] + "..."
		}

		avgMs := int64(0)
		if svc.SpanCount > 0 {
			avgMs = (svc.TotalDuration / int64(svc.SpanCount)) / 1000
		}

		errPart := ""
		if svc.ErrorCount > 0 {
			errPart = fmt.Sprintf("  errors: %d", svc.ErrorCount)
		}

		fmt.Printf("  %-24s spans: %-5d%s  avg: %dms  ops: [%s]\n",
			svc.Name, svc.SpanCount, errPart, avgMs, opStr)
	}

	if len(edges) == 0 {
		return nil
	}

	// Sort edges.
	edgeList := make([]*topologyEdge, 0, len(edges))
	for _, edge := range edges {
		edgeList = append(edgeList, edge)
	}
	sort.Slice(edgeList, func(i, j int) bool {
		a, b := edgeList[i], edgeList[j]
		if a.ParentService != b.ParentService {
			return a.ParentService < b.ParentService
		}
		return a.ChildService < b.ChildService
	})

	fmt.Println()
	fmt.Printf("Dependencies (%d):\n", len(edgeList))
	for _, edge := range edgeList {
		fmt.Printf("  %s → %s  (%d spans)\n",
			edge.ParentService, edge.ChildService, edge.SpanCount)
	}
	return nil
}
