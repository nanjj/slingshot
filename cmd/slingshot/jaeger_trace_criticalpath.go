package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nanjj/slingshot/internal/i18n"
)

// cmdJaegerTraceCriticalPath 实现 "slingshot jaeger trace critical-path <traceID>"。
type cmdJaegerTraceCriticalPath struct {
	global     *cmdGlobal
	jaeger     *cmdJaeger
	trace      *cmdJaegerTrace
	jsonOutput bool
}

func (c *cmdJaegerTraceCriticalPath) command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "critical-path <traceID>"
	cmd.Short = i18n.G("Show trace critical path")
	cmd.Long = i18n.G(`Identify the critical path (longest chain) through the trace spans.

Walks from the root span down to the leaf, at each level picking the
child with the longest duration. The result is the chain of spans most
responsible for end-to-end latency — the first thing to optimise.`)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().BoolVar(&c.jsonOutput, "json", false, i18n.G("Output as JSON"))
	cmd.RunE = c.run
	return cmd
}

func (c *cmdJaegerTraceCriticalPath) run(cmd *cobra.Command, args []string) error {
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
	return printCriticalPath(trace, c.jsonOutput)
}

// cpNode represents one span in the critical path tree.
type cpNode struct {
	SpanID    string   `json:"spanID"`
	Service   string   `json:"service"`
	Operation string   `json:"operation"`
	DurationMs int64   `json:"durationMs"`
	Children  []*cpNode `json:"children,omitempty"`
}

func printCriticalPath(trace jaegerTraceData, asJSON bool) error {
	if len(trace.Spans) == 0 {
		fmt.Println(i18n.G("No spans in trace"))
		return nil
	}

	// Index spans by ID.
	spanMap := make(map[string]jaegerSpan, len(trace.Spans))
	for _, s := range trace.Spans {
		spanMap[s.SpanID] = s
	}

	// Build parent→children map. Spans with no parent in this trace are roots.
	children := make(map[string][]jaegerSpan) // key: parentSpanID or "" for roots
	for _, s := range trace.Spans {
		parentID := parentSpanID(s, spanMap)
		children[parentID] = append(children[parentID], s)
	}

	roots := children[""]
	if len(roots) == 0 {
		fmt.Println(i18n.G("No root spans found in trace"))
		return nil
	}

	// For each root compute the critical chain; pick the longest by root duration.
	var bestRoot jaegerSpan
	var bestPath []jaegerSpan
	for i, root := range roots {
		path := walkCriticalPath(root.SpanID, children, spanMap)
		if i == 0 || root.Duration > bestRoot.Duration {
			bestRoot = root
			bestPath = path
		}
	}

	// Build tree for display.
	tree := buildCPTree(bestPath, trace.Processes, spanMap)

	if asJSON {
		out, err := json.MarshalIndent(tree, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}

	printCPTree(tree, 0)
	fmt.Printf("\nTotal: %dms\n", bestRoot.durationMs())
	return nil
}

// parentSpanID returns the ID of the first CHILD_OF parent found in the span map,
// or "" if none (making this span a root).
func parentSpanID(s jaegerSpan, spanMap map[string]jaegerSpan) string {
	for _, ref := range s.References {
		if ref.RefType == "CHILD_OF" {
			if _, ok := spanMap[ref.SpanID]; ok {
				return ref.SpanID
			}
		}
	}
	return ""
}

// walkCriticalPath walks from the given spanID down, always picking the child
// with the longest duration. Returns the chain as a slice.
func walkCriticalPath(spanID string, children map[string][]jaegerSpan, spanMap map[string]jaegerSpan) []jaegerSpan {
	var path []jaegerSpan
	current := spanID
	for {
		span, ok := spanMap[current]
		if !ok {
			break
		}
		path = append(path, span)

		childList := children[current]
		if len(childList) == 0 {
			break
		}
		best := childList[0]
		for _, c := range childList[1:] {
			if c.Duration > best.Duration {
				best = c
			}
		}
		current = best.SpanID
	}
	return path
}

// buildCPTree converts a flat critical-path span slice into a nested cpNode tree.
func buildCPTree(path []jaegerSpan, processes map[string]jaegerProcess, spanMap map[string]jaegerSpan) *cpNode {
	if len(path) == 0 {
		return nil
	}
	root := &cpNode{
		SpanID:     path[0].SpanID,
		Service:    path[0].serviceName(processes),
		Operation:  path[0].OperationName,
		DurationMs: path[0].durationMs(),
	}
	node := root
	for i := 1; i < len(path); i++ {
		child := &cpNode{
			SpanID:     path[i].SpanID,
			Service:    path[i].serviceName(processes),
			Operation:  path[i].OperationName,
			DurationMs: path[i].durationMs(),
		}
		node.Children = []*cpNode{child}
		node = child
	}
	return root
}

// printCPTree prints a nested cpNode tree with indentation and tree guides.
func printCPTree(node *cpNode, depth int) {
	indent := ""
	if depth > 0 {
		indent = strings.Repeat("  ", depth-1) + "└─ "
	}
	label := node.Service + "." + node.Operation
	fmt.Printf("%s%-40s [%4dms]\n", indent, label, node.DurationMs)
	for _, child := range node.Children {
		printCPTree(child, depth+1)
	}
}
