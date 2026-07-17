// Package arguments parses tool-call arguments from the CLI's three
// mutually-exclusive input modes into a JSON object.
package arguments

import (
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/jstevewhite/mcpctl/internal/apperror"
)

// Parse builds the arguments object. Exactly one of jsonStr, jsonFile, or
// argKVs may be provided; none yields (nil, nil). stdin is used when jsonFile
// is "-".
func Parse(jsonStr, jsonFile string, argKVs []string, stdin io.Reader) (map[string]any, error) {
	modes := 0
	if jsonStr != "" {
		modes++
	}
	if jsonFile != "" {
		modes++
	}
	if len(argKVs) > 0 {
		modes++
	}
	if modes > 1 {
		return nil, apperror.Usage("use only one of --json, --json-file, or --arg")
	}

	switch {
	case jsonStr != "":
		return decodeObject([]byte(jsonStr))
	case jsonFile != "":
		data, err := readFileOrStdin(jsonFile, stdin)
		if err != nil {
			return nil, err
		}
		return decodeObject(data)
	case len(argKVs) > 0:
		return parseArgs(argKVs)
	default:
		return nil, nil
	}
}

func readFileOrStdin(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, apperror.Wrap(apperror.KindInvalidArgs, err, "read arguments from stdin")
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindInvalidArgs, err, "read arguments file %q", path)
	}
	return data, nil
}

// decodeObject requires the JSON to be a top-level object.
func decodeObject(data []byte) (map[string]any, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, apperror.Wrap(apperror.KindInvalidArgs, err, "parse JSON arguments")
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, apperror.New(apperror.KindInvalidArgs, "arguments must be a JSON object, got %T", v)
	}
	return obj, nil
}

// parseArgs parses repeated KEY=VALUE pairs. Each value is decoded as JSON if
// it is valid JSON, otherwise treated as a string. Numeric-looking values
// therefore decode as numbers (e.g. version=1.10 -> 1.1); quote as a JSON
// string ("1.10") or use --json to force text.
func parseArgs(kvs []string) (map[string]any, error) {
	out := make(map[string]any, len(kvs))
	for _, kv := range kvs {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			return nil, apperror.New(apperror.KindInvalidArgs, "invalid --arg %q: expected KEY=VALUE", kv)
		}
		key := kv[:eq]
		if key == "" {
			return nil, apperror.New(apperror.KindInvalidArgs, "invalid --arg %q: empty key", kv)
		}
		if _, dup := out[key]; dup {
			return nil, apperror.New(apperror.KindInvalidArgs, "duplicate --arg key %q", key)
		}
		raw := kv[eq+1:]
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			out[key] = parsed
		} else {
			out[key] = raw
		}
	}
	return out, nil
}
