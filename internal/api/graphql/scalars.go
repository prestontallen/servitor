package graphql

import (
	"encoding/json"
	"fmt"
	"io"
)

// RawJSON is a JSON scalar whose bytes are served as-is, so a document the
// store already built (the ctx aggregate) keeps its key order and nulls.
type RawJSON json.RawMessage

func (r RawJSON) MarshalGQL(w io.Writer) {
	if len(r) == 0 {
		io.WriteString(w, "null")
		return
	}
	w.Write(r)
}

// UnmarshalGQL exists to satisfy the scalar contract; no argument takes JSON.
func (r *RawJSON) UnmarshalGQL(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("JSON scalar: %w", err)
	}
	*r = b
	return nil
}
