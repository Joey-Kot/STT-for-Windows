// Copyright (C) 2026 Joey Kot <joey.kot.x@gmail.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed WITHOUT ANY WARRANTY; without even the
// implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See <https://www.gnu.org/licenses/> for more details.

package jsonpath

import "testing"

func TestExtractByPath(t *testing.T) {
	root := map[string]interface{}{
		"text": "hello",
		"data": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"value": "a"},
				map[string]interface{}{"value": "b"},
			},
		},
		"results": []interface{}{
			map[string]interface{}{
				"alternatives": []interface{}{
					map[string]interface{}{"transcript": "ok"},
				},
			},
		},
	}

	if v, ok := ExtractByPath(root, "data.items[1].value"); !ok || v != "b" {
		t.Fatalf("expected b, got %v (ok=%v)", v, ok)
	}
	if v, ok := ExtractByPath(root, "results[0].alternatives[0].transcript"); !ok || v != "ok" {
		t.Fatalf("expected ok, got %v (ok=%v)", v, ok)
	}
	if _, ok := ExtractByPath(root, "data.items[99].value"); ok {
		t.Fatalf("expected not found")
	}
}

func TestParseKeyAndIndexes(t *testing.T) {
	key, idxs, err := ParseKeyAndIndexes("foo[0][1]")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if key != "foo" || len(idxs) != 2 || idxs[0] != 0 || idxs[1] != 1 {
		t.Fatalf("unexpected parse result: key=%s idxs=%v", key, idxs)
	}
}

func TestExtractTextFromResponse(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		textPath string
		want     string
	}{
		{name: "configured path", body: []byte(`{"data":{"items":[{"transcript":"hello"}]}}`), textPath: "data.items[0].transcript", want: "hello"},
		{name: "default text string", body: []byte(`{"text":"fallback"}`), want: "fallback"},
		{name: "default text integer", body: []byte(`{"text":42}`), want: "42"},
		{name: "default text float", body: []byte(`{"text":42.5}`), want: "42.5"},
		{name: "default text bool", body: []byte(`{"text":true}`), want: "true"},
		{name: "first non-empty string field", body: []byte(`{"empty":"","other":"value"}`), want: "value"},
		{name: "invalid json", body: []byte(`not-json`), textPath: "text", want: ""},
		{name: "missing configured path falls back", body: []byte(`{"text":"fallback"}`), textPath: "missing.path", want: "fallback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractTextFromResponse(tt.body, tt.textPath); got != tt.want {
				t.Fatalf("ExtractTextFromResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractByPathCoercesScalarValues(t *testing.T) {
	root := map[string]interface{}{
		"int":   float64(7),
		"float": float64(7.5),
		"bool":  true,
	}

	tests := map[string]string{
		"int":   "7",
		"float": "7.5",
		"bool":  "true",
	}
	for path, want := range tests {
		if got, ok := ExtractByPath(root, path); !ok || got != want {
			t.Fatalf("ExtractByPath(%q) = %q, %v; want %q, true", path, got, ok, want)
		}
	}
}

func TestExtractByPathRejectsInvalidPaths(t *testing.T) {
	root := map[string]interface{}{
		"items": []interface{}{"zero"},
		"nested": map[string]interface{}{
			"value": map[string]interface{}{"notScalar": []interface{}{"x"}},
		},
	}

	paths := []string{
		"",
		"items[-1]",
		"items[bad]",
		"items[]",
		"items[0",
		"items[0]extra",
		"items.value",
		"missing.value",
		"nested.value.notScalar",
	}
	for _, path := range paths {
		if got, ok := ExtractByPath(root, path); ok {
			t.Fatalf("ExtractByPath(%q) = %q, true; want not found", path, got)
		}
	}
}

func TestParseKeyAndIndexesAdditionalForms(t *testing.T) {
	key, idxs, err := ParseKeyAndIndexes("[2]")
	if err != nil {
		t.Fatalf("ParseKeyAndIndexes([2]) error: %v", err)
	}
	if key != "" || len(idxs) != 1 || idxs[0] != 2 {
		t.Fatalf("ParseKeyAndIndexes([2]) = key %q indexes %v, want empty key and [2]", key, idxs)
	}

	if _, _, err := ParseKeyAndIndexes(""); err == nil {
		t.Fatalf("ParseKeyAndIndexes(empty) succeeded, want error")
	}
}
