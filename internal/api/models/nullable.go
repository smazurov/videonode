package models

import (
	"bytes"
	"encoding/json"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

// Nullable is a three-state PATCH-field wrapper: omitted, explicit null, or a value.
type Nullable[T any] struct {
	Sent  bool
	Null  bool
	Value T // valid only when Sent && !Null
}

// UnmarshalJSON detects null vs omitted.
func (o *Nullable[T]) UnmarshalJSON(b []byte) error {
	if len(b) > 0 {
		o.Sent = true
		if bytes.Equal(b, []byte("null")) {
			o.Null = true
			return nil
		}
		return json.Unmarshal(b, &o.Value)
	}
	return nil
}

// Schema renders a nullable OpenAPI schema via anyOf [T, null].
func (o Nullable[T]) Schema(r huma.Registry) *huma.Schema {
	s := r.Schema(reflect.TypeOf(o.Value), true, "")
	return &huma.Schema{
		AnyOf: []*huma.Schema{
			s,
			{Type: "null"},
		},
	}
}
