//go:build planv2_tests

// Shared test helpers for the planv2 API CRUD tests. These avoid
// pulling in Huma's full registration apparatus while still exercising
// the JSON contract end-to-end. Real B5/B6/B7 routes register handlers
// via Huma directly; once those land, tests will swap this in-process
// router for a humatest.TestAPI.
package api

import (
	"encoding/json"
)

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func mustUnmarshal(s string, v any) {
	if err := json.Unmarshal([]byte(s), v); err != nil {
		panic(err)
	}
}
