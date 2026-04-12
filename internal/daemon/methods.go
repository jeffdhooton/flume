package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jeffdhooton/flume/internal/rpc"
	"github.com/jeffdhooton/flume/internal/store"
)

func (d *Daemon) registerMethods() {
	d.server.Register("ping", func(_ context.Context, _ json.RawMessage) (any, error) {
		return map[string]any{"ok": true, "pid": os.Getpid()}, nil
	})
	d.server.Register("status", d.handleStatus)
	d.server.Register("shutdown", d.handleShutdown)
	d.server.Register("requests", d.handleRequests)
	d.server.Register("request", d.handleRequest)
}

// StatusResult is returned by the status RPC method.
type StatusResult struct {
	PID         int    `json:"pid"`
	Uptime      string `json:"uptime"`
	ProxyPort   int    `json:"proxy_port"`
	TargetAddr  string `json:"target_addr"`
	RequestCount int   `json:"request_count"`
}

func (d *Daemon) handleStatus(_ context.Context, _ json.RawMessage) (any, error) {
	count := 0
	if d.store != nil {
		count = d.store.Count()
	}
	return &StatusResult{
		PID:         os.Getpid(),
		Uptime:      fmt.Sprintf("%s", time.Since(d.startAt).Round(time.Second)),
		ProxyPort:   d.config.ProxyPort,
		TargetAddr:  d.config.TargetAddr,
		RequestCount: count,
	}, nil
}

func (d *Daemon) handleShutdown(_ context.Context, _ json.RawMessage) (any, error) {
	go func() {
		time.Sleep(50 * time.Millisecond)
		d.mu.Lock()
		ln := d.listener
		d.mu.Unlock()
		if ln != nil {
			_ = ln.Close()
		}
	}()
	return map[string]any{"ok": true}, nil
}

func (d *Daemon) handleRequests(_ context.Context, raw json.RawMessage) (any, error) {
	var f store.ListFilter
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, &rpc.Error{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
	}
	if d.store == nil {
		return []store.RequestSummary{}, nil
	}
	results, err := d.store.List(f)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []store.RequestSummary{}
	}
	return results, nil
}

// RequestParams identifies a single request by ID.
type RequestParams struct {
	ID string `json:"id"`
}

func (d *Daemon) handleRequest(_ context.Context, raw json.RawMessage) (any, error) {
	var p RequestParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.CodeInvalidParams, Message: err.Error()}
	}
	if p.ID == "" {
		return nil, &rpc.Error{Code: rpc.CodeInvalidParams, Message: "id is required"}
	}
	if d.store == nil {
		return nil, &rpc.Error{Code: rpc.CodeInternalError, Message: "store not initialized"}
	}
	req, err := d.store.Get(p.ID)
	if err != nil {
		return nil, err
	}
	return req, nil
}
