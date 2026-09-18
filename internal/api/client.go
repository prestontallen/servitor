package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prestontallen/servitor/internal/store"
)

// HTTPClient implements Service over the HTTP transport. It exists so the
// CLI (and MCP server) are provably thin: they depend on Service and talk
// HTTP, nothing more. All state lives server-side.
type HTTPClient struct {
	Base  string       // e.g. http://localhost:8181
	HTTP  *http.Client // nil = default
	Actor string       // attested actor, sent as X-Servitor-Actor
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body any, out any) error {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Base, "/")+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Actor != "" {
		req.Header.Set("X-Servitor-Actor", c.Actor)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return &APIError{Code: "unreachable", Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var er struct {
			Error APIError `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&er) == nil && er.Error.Code != "" {
			return &er.Error
		}
		return &APIError{Code: "internal", Message: fmt.Sprintf("http %d", resp.StatusCode)}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *HTTPClient) Ctx(ctx context.Context, ref string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/ticket/"+ref, nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *HTTPClient) Board(ctx context.Context) ([]store.Card, error) {
	var cards []store.Card
	if err := c.do(ctx, http.MethodGet, "/api/board", nil, &cards); err != nil {
		return nil, err
	}
	return cards, nil
}

// Arcs lists arcs (tickets with members) with derived rollups.
func (c *HTTPClient) Arcs(ctx context.Context) ([]store.ArcSummary, error) {
	var arcs []store.ArcSummary
	if err := c.do(ctx, http.MethodGet, "/api/arcs", nil, &arcs); err != nil {
		return nil, err
	}
	return arcs, nil
}

// List reaches every ticket status, including done and dropped. The
// returned cards use the same additive-keys shape as Board.
func (c *HTTPClient) List(ctx context.Context, f store.ListFilter) ([]store.Card, error) {
	q := url.Values{}
	for _, st := range f.Statuses {
		q.Add("status", st)
	}
	if f.Query != "" {
		q.Set("q", f.Query)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	var cards []store.Card
	path := "/api/tickets"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &cards); err != nil {
		return nil, err
	}
	return cards, nil
}

func (c *HTTPClient) History(ctx context.Context, ref string, limit int) ([]store.LedgerEvent, error) {
	var evs []store.LedgerEvent
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/ticket/%s/history?limit=%d", ref, limit), nil, &evs); err != nil {
		return nil, err
	}
	return evs, nil
}

func (c *HTTPClient) Append(ctx context.Context, cmd WriteCmd) (AppendResult, error) {
	var res AppendResult
	if err := c.do(ctx, http.MethodPost, "/api/events", cmd, &res); err != nil {
		return AppendResult{}, err
	}
	return res, nil
}

// Feedback lists feedback-kind ledger events across all tickets.
func (c *HTTPClient) Feedback(ctx context.Context, f FeedbackFilter) ([]store.LedgerEvent, error) {
	q := "/api/feedback?"
	if f.Since != nil {
		q += "since=" + f.Since.Format(time.RFC3339) + "&"
	}
	if f.Source != "" {
		q += "source=" + f.Source + "&"
	}
	q += fmt.Sprintf("limit=%d", f.Limit)
	var evs []store.LedgerEvent
	if err := c.do(ctx, http.MethodGet, q, nil, &evs); err != nil {
		return nil, err
	}
	return evs, nil
}

// Subscribe consumes the SSE stream. On `event: resync` the channel closes;
// callers replay past their watermark via History and re-subscribe.
func (c *HTTPClient) Subscribe(ctx context.Context) (Subscription, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(c.Base, "/")+"/api/events/stream", nil)
	if err != nil {
		return Subscription{}, err
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Subscription{}, &APIError{Code: "unreachable", Message: err.Error()}
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return Subscription{}, &APIError{Code: "internal", Message: fmt.Sprintf("stream http %d", resp.StatusCode)}
	}
	ch := make(chan Change, 64)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		var data string
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			switch {
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			case line == "" && data != "":
				var chg Change
				if json.Unmarshal([]byte(data), &chg) == nil {
					select {
					case ch <- chg:
					case <-ctx.Done():
						return
					}
				}
				data = ""
			case line == "event: resync":
				return
			}
		}
	}()
	return Subscription{Changes: ch, Cancel: func() {}}, nil
}

func (c *HTTPClient) Analytics(ctx context.Context, days int) ([]DayBucket, error) {
	var out []DayBucket
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/analytics?days=%d", days), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *HTTPClient) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/api/healthz", nil, nil)
}

var _ Service = (*HTTPClient)(nil)
