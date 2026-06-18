package main

// jaegerTraceResponse is the top-level response from /api/traces/{id}.
type jaegerTraceResponse struct {
	Data []jaegerTraceData `json:"data"`
}

type jaegerTraceData struct {
	TraceID   string                 `json:"traceID"`
	Spans     []jaegerSpan           `json:"spans"`
	Processes map[string]jaegerProcess `json:"processes"`
	Warnings  []string               `json:"warnings"`
}

type jaegerSpan struct {
	SpanID        string      `json:"spanID"`
	OperationName string      `json:"operationName"`
	StartTime     int64       `json:"startTime"`
	Duration      int64       `json:"duration"` // microseconds
	ProcessID     string      `json:"processID"`
	References    []jaegerRef `json:"references"`
	Tags          []jaegerTag `json:"tags"`
	Logs          []jaegerLog `json:"logs"`
}

type jaegerRef struct {
	RefType string `json:"refType"` // CHILD_OF, FOLLOWS_FROM
	TraceID string `json:"traceID"`
	SpanID  string `json:"spanID"`
}

type jaegerProcess struct {
	ServiceName string      `json:"serviceName"`
	Tags        []jaegerTag `json:"tags"`
}

type jaegerTag struct {
	Key   string      `json:"key"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type jaegerLog struct {
	Timestamp int64       `json:"timestamp"`
	Fields    []jaegerTag `json:"fields"`
}

// serviceName returns the service name for this span, resolving from processes.
func (s *jaegerSpan) serviceName(processes map[string]jaegerProcess) string {
	if p, ok := processes[s.ProcessID]; ok {
		return p.ServiceName
	}
	return "unknown"
}

// durationMs returns the span duration in milliseconds.
func (s *jaegerSpan) durationMs() int64 {
	return s.Duration / 1000
}

// hasError checks whether the span has an error=true tag.
func (s *jaegerSpan) hasError() bool {
	for _, tag := range s.Tags {
		if tag.Key == "error" {
			switch v := tag.Value.(type) {
			case bool:
				if v {
					return true
				}
			case string:
				if v == "true" {
					return true
				}
			}
		}
	}
	return false
}
