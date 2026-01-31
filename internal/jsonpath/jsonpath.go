package jsonpath

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ExtractTextFromResponse extracts text from JSON response using provided path.
func ExtractTextFromResponse(body []byte, textPath string) string {
	var root interface{}
	if err := json.Unmarshal(body, &root); err != nil {
		return ""
	}

	if textPath != "" {
		if v, ok := ExtractByPath(root, textPath); ok {
			return v
		}
	}

	if m, ok := root.(map[string]interface{}); ok {
		if v, exists := m["text"]; exists {
			switch s := v.(type) {
			case string:
				return s
			case float64:
				if s == float64(int64(s)) {
					return fmt.Sprintf("%d", int64(s))
				}
				return fmt.Sprintf("%v", s)
			case bool:
				return fmt.Sprintf("%v", s)
			default:
			}
		}
		for _, val := range m {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// ExtractByPath extracts a string value from a JSON-parsed structure using a dot-separated path.
func ExtractByPath(root interface{}, path string) (string, bool) {
	if path == "" {
		return "", false
	}
	parts := strings.Split(path, ".")
	cur := root
	for _, part := range parts {
		key, idxs, err := ParseKeyAndIndexes(part)
		if err != nil {
			return "", false
		}

		if key != "" {
			m, ok := cur.(map[string]interface{})
			if !ok {
				return "", false
			}
			next, exists := m[key]
			if !exists {
				return "", false
			}
			cur = next
		}

		for _, idx := range idxs {
			arr, ok := cur.([]interface{})
			if !ok {
				return "", false
			}
			if idx < 0 || idx >= len(arr) {
				return "", false
			}
			cur = arr[idx]
		}
	}

	switch v := cur.(type) {
	case string:
		return v, true
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v)), true
		}
		return fmt.Sprintf("%v", v), true
	case bool:
		return fmt.Sprintf("%v", v), true
	default:
		return "", false
	}
}

// ParseKeyAndIndexes parses a token like "foo[0][1]" or "[0]" or "bar" into base key and indexes.
func ParseKeyAndIndexes(token string) (string, []int, error) {
	if token == "" {
		return "", nil, fmt.Errorf("empty token")
	}
	idxs := []int{}
	br := strings.Index(token, "[")
	var key string
	if br == -1 {
		key = token
		return key, idxs, nil
	}
	key = token[:br]
	rest := token[br:]
	for len(rest) > 0 {
		if !strings.HasPrefix(rest, "[") {
			return "", nil, fmt.Errorf("invalid index syntax in %s", token)
		}
		closePos := strings.Index(rest, "]")
		if closePos == -1 {
			return "", nil, fmt.Errorf("missing closing ] in %s", token)
		}
		numStr := rest[1:closePos]
		if numStr == "" {
			return "", nil, fmt.Errorf("empty index in %s", token)
		}
		n, err := strconv.Atoi(numStr)
		if err != nil {
			return "", nil, fmt.Errorf("invalid index '%s' in %s", numStr, token)
		}
		idxs = append(idxs, n)
		rest = rest[closePos+1:]
	}
	return key, idxs, nil
}
