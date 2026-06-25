package mcp

import (
	"encoding/json"
	"testing"
)

func TestTryUnmarshalJSONStr_Object(t *testing.T) {
	// A JSON-encoded string containing a map → should be converted
	input := `{"path":[{"type":"function","childIndex":0}]}`
	result, ok := tryUnmarshalJSONStr(input)
	if !ok {
		t.Fatal("expected ok=true for JSON object string")
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	arr, ok := m["path"].([]any)
	if !ok {
		t.Fatalf("expected path to be []any, got %T", m["path"])
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 element in path, got %d", len(arr))
	}
}

func TestTryUnmarshalJSONStr_NestedSelector(t *testing.T) {
	// Real-world case: LLM sends selector as double-encoded JSON string
	input := `{"path":[{"type":"pair","value":"key"}],"point":[1,0]}`
	result, ok := tryUnmarshalJSONStr(input)
	if !ok {
		t.Fatal("expected ok=true for nested JSON object")
	}
	m := result.(map[string]any)
	if _, exists := m["point"]; !exists {
		t.Fatal("expected point key in parsed result")
	}
}

func TestTryUnmarshalJSONStr_PlainString(t *testing.T) {
	// A plain string → should NOT be converted
	result, ok := tryUnmarshalJSONStr("function:Hello")
	if ok {
		t.Fatal("expected ok=false for plain string")
	}
	if result != "function:Hello" {
		t.Fatalf("expected original value, got %v", result)
	}
}

func TestTryUnmarshalJSONStr_Number(t *testing.T) {
	// A number (not a string) → should NOT be converted
	result, ok := tryUnmarshalJSONStr(42)
	if ok {
		t.Fatal("expected ok=false for non-string")
	}
	if result != 42 {
		t.Fatalf("expected original value, got %v", result)
	}
}

func TestTryUnmarshalJSONStr_EmptyString(t *testing.T) {
	// Empty string → should NOT be converted
	result, ok := tryUnmarshalJSONStr("")
	if ok {
		t.Fatal("expected ok=false for empty string")
	}
	if result != "" {
		t.Fatalf("expected original value, got %v", result)
	}
}

func TestTryUnmarshalJSONStr_JSONArray(t *testing.T) {
	// A JSON array string → should NOT be converted (only maps)
	input := `[1, 2, 3]`
	result, ok := tryUnmarshalJSONStr(input)
	if ok {
		t.Fatal("expected ok=false for JSON array, only objects should be converted")
	}
	if result != input {
		t.Fatalf("expected original value, got %v", result)
	}
}

func TestTryUnmarshalJSONStr_JSONScalar(t *testing.T) {
	// A JSON number string → should NOT be converted
	result, ok := tryUnmarshalJSONStr(`"hello"`)
	if ok {
		t.Fatal("expected ok=false for scalar JSON (string)")
	}
	if result != `"hello"` {
		t.Fatalf("expected original value, got %v", result)
	}
}

func TestFixDoubleEncodedArgs_NoChange(t *testing.T) {
	raw := json.RawMessage(`{"file":"main.go","mode":"insert","pos":10,"text":"hello"}`)
	fixed, changed, err := fixDoubleEncodedArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("expected no change for clean args")
	}
	if string(fixed) != string(raw) {
		t.Fatalf("expected same JSON, got %s", string(fixed))
	}
}

func TestFixDoubleEncodedArgs_WithFix(t *testing.T) {
	// selector is double-encoded: a string containing JSON
	raw := json.RawMessage(`{"file":"main.go","selector":"{\"path\":[{\"type\":\"function\",\"childIndex\":0}]}","mode":"replace","text":"return 42"}`)
	fixed, changed, err := fixDoubleEncodedArgs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected change for double-encoded selector")
	}
	var args map[string]any
	if err := json.Unmarshal(fixed, &args); err != nil {
		t.Fatalf("failed to unmarshal fixed json: %v", err)
	}
	// selector should now be a map, not a string
	if _, ok := args["selector"].(string); ok {
		t.Fatal("expected selector to be a map after fix, but it's still a string")
	}
	if _, ok := args["selector"].(map[string]any); !ok {
		t.Fatalf("expected selector to be map[string]any, got %T", args["selector"])
	}
}

func TestFixDoubleEncodedArgs_BadJSON(t *testing.T) {
	// Invalid JSON → should return error
	raw := json.RawMessage(`{invalid json}`)
	_, _, err := fixDoubleEncodedArgs(raw)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
