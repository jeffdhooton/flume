package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type callLogEntry struct {
	Timestamp string `json:"ts"`
	Tool      string `json:"tool"`
	Repo      string `json:"repo,omitempty"`
	Results   int    `json:"results"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

func logCall(entry callLogEntry) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".flume", "logs")
	_ = os.MkdirAll(dir, 0o755)

	f, err := os.OpenFile(filepath.Join(dir, "mcp-calls.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(f, "%s\n", b)
}

// extractResultCount best-effort counts the items returned by a tool. If the
// JSON is a top-level array we use its length; if it's an object exposing
// `total` or `count` we use that; otherwise -1 (unknown).
func extractResultCount(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		return len(arr)
	}
	var obj struct {
		Total *int `json:"total"`
		Count *int `json:"count"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if obj.Total != nil {
			return *obj.Total
		}
		if obj.Count != nil {
			return *obj.Count
		}
	}
	return -1
}

