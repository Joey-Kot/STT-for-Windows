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
