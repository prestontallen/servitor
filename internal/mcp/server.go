// Package mcp exposes the Service interface as an MCP (Model Context
// Protocol) server over stdio: JSON-RPC 2.0, one request per line-escaped
// message per the MCP framing rules. Hand-rolled minimal protocol: the
// tool surface is small and stable, and an SDK dependency buys nothing here.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

// tool is one MCP tool definition: name, human description, and JSON schema.
type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func tools() []tool {
	return []tool{
		{
			Name:        "servitor_ctx",
			Description: "Read the whole ticket aggregate (status, gates, criteria, plan, decisions, notes, questions, links) by ticket ULID or slug.",
			InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "ref": {"type": "string", "description": "ticket ULID or slug"}
  },
  "required": ["ref"]
}`),
		},
		{
			Name:        "servitor_board",
			Description: "List queued/active/blocked tickets in rank order with derived card word.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
		{
			Name:        "servitor_history",
			Description: "Full event timeline for one ticket (the ledger, ordered).",
			InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "ref":   {"type": "string", "description": "ticket ULID or slug"},
    "limit": {"type": "integer", "description": "max events (default 10000)"}
  },
  "required": ["ref"]
}`),
		},
		{
			Name:        "servitor_append",
			Description: "Append one event to a ticket (the only write path). Kinds: ticket.create (payload: slug,title,rank), status.set (status: queued|active|blocked|done|dropped; blocked requires on), field.set (field + v; omit v to make the field absent), gate (contract_approved requires a human: actor), decision (what, why), note (v), flow.set (nodes:[{id,label,state}] with state queued|active|blocked|done and edges:[{from,to}] — a whole-graph snapshot; the ticket GUI renders the latest one as a flowchart), subitem.add (kind,body,rank), subitem.set (ulid prefix, body/state), subitem.rank (ulid prefix, rank). Unknown kinds are stored verbatim.",
			InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "ticket":  {"type": "string", "description": "ticket ULID or slug (omit for ticket.create; the new ticket's ULID goes in payload.ulid)"},
    "kind":    {"type": "string"},
    "payload": {"type": "object"},
    "actor":   {"type": "string", "description": "human:<name> | agent:<id> | system"}
  },
  "required": ["kind", "payload", "actor"]
}`),
		},
	}
}

// Server wires a Service into stdio JSON-RPC.
type Server struct {
	Service api.Service
	In      io.Reader
	Out     io.Writer
	Err     io.Writer // human-readable logs; never stdout (stdout is the protocol)
}

// Serve reads JSON-RPC messages until EOF. Blocks; run in the foreground.
func (s *Server) Serve(ctx context.Context) error {
	sc := bufio.NewScanner(s.In)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(s.Out)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req struct {
			ID     *json.RawMessage `json:"id"`
			Method string           `json:"method"`
			Params json.RawMessage  `json:"params"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			// malformed request: notification-safe error only if it had an id
			fmt.Fprintln(s.Err, "mcp: bad frame:", err)
			continue
		}
		if req.Method == "" || req.ID == nil {
			continue // notification
		}
		result, rpcErr := s.dispatch(ctx, req.Method, req.Params)
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if rpcErr != nil {
			resp["error"] = rpcErr
		} else {
			resp["result"] = result
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

func rpcError(code int, msg string) map[string]any {
	return map[string]any{"code": code, "message": msg}
}

const (
	codeParse     = -32700
	codeInvalid   = -32602
	codeMethod    = -32601
	codeInternal  = -32603
	codeToolError = -32000 // tool-level (domain) failure
)

func (s *Server) dispatch(ctx context.Context, method string, params json.RawMessage) (any, *rpcErrShape) {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "servitor", "version": "0.1.0"},
		}, nil
	case "notifications/initialized":
		return nil, nil
	case "tools/list":
		return map[string]any{"tools": tools()}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcErrShape{codeInvalid, "bad tools/call params"}
		}
		text, err := s.callTool(ctx, p.Name, p.Arguments)
		if err != nil {
			var ae *api.APIError
			if errors.As(err, &ae) {
				return nil, &rpcErrShape{codeToolError, fmt.Sprintf("%s: %s", ae.Code, ae.Message)}
			}
			return nil, &rpcErrShape{codeInternal, err.Error()}
		}
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
		}, nil
	default:
		return nil, &rpcErrShape{codeMethod, "unknown method " + method}
	}
}

type rpcErrShape struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// errorsAs removed: use errors.As directly.

func (s *Server) callTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	switch name {
	case "servitor_ctx":
		var p struct {
			Ref string `json:"ref"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return "", err
		}
		doc, err := s.Service.Ctx(ctx, p.Ref)
		if err != nil {
			return "", err
		}
		return string(doc), nil

	case "servitor_board":
		cards, err := s.Service.Board(ctx)
		if err != nil {
			return "", err
		}
		b, err := json.Marshal(cards)
		return string(b), err

	case "servitor_history":
		var p struct {
			Ref   string `json:"ref"`
			Limit int    `json:"limit"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return "", err
		}
		evs, err := s.Service.History(ctx, p.Ref, p.Limit)
		if err != nil {
			return "", err
		}
		b, err := json.Marshal(evs)
		return string(b), err

	case "servitor_append":
		var p struct {
			Ticket  string         `json:"ticket"`
			Kind    string         `json:"kind"`
			Payload map[string]any `json:"payload"`
			Actor   string         `json:"actor"`
			Session string         `json:"session"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return "", err
		}
		// ticket.create carries its own ULID in the payload
		ticket := p.Ticket
		if ticket == "" && p.Kind == "ticket.create" {
			ticket, _ = p.Payload["ulid"].(string)
			if ticket == "" {
				ticket = store.NewULID()
				p.Payload["ulid"] = ticket
			}
		}
		res, err := s.Service.Append(ctx, api.WriteCmd{
			Ticket: ticket, Kind: p.Kind, Payload: p.Payload,
			Actor: p.Actor, Session: p.Session,
		})
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(res)
		return string(b), nil

	default:
		return "", fmt.Errorf("unknown tool %s", name)
	}
}
