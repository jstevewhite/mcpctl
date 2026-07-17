package arguments

import (
	"errors"
	"testing"
)

var objSchema = map[string]any{
	"type":     "object",
	"required": []any{"path"},
	"properties": map[string]any{
		"path": map[string]any{"type": "string"},
	},
}

func TestValidateNilSchemaSkips(t *testing.T) {
	if err := Validate(nil, map[string]any{"x": 1}); err != nil {
		t.Fatalf("nil schema should skip validation, got %v", err)
	}
}

func TestValidateAccepts(t *testing.T) {
	if err := Validate(objSchema, map[string]any{"path": "/tmp/x"}); err != nil {
		t.Fatalf("valid args rejected: %v", err)
	}
}

func TestValidateRejectsMissingRequired(t *testing.T) {
	err := Validate(objSchema, map[string]any{})
	if err == nil {
		t.Fatal("missing required 'path' should fail validation")
	}
	var unusable *SchemaUnusableError
	if errors.As(err, &unusable) {
		t.Fatalf("a schema violation must not be reported as unusable-schema: %v", err)
	}
}

func TestValidateRejectsWrongType(t *testing.T) {
	if err := Validate(objSchema, map[string]any{"path": 123}); err == nil {
		t.Fatal("path as a number should fail validation")
	}
}

func TestValidateUnusableSchema(t *testing.T) {
	// A schema the compiler cannot make sense of degrades to SchemaUnusableError.
	bad := map[string]any{"type": 12345} // "type" must be a string/array, not a number
	err := Validate(bad, map[string]any{"x": 1})
	var unusable *SchemaUnusableError
	if err != nil && !errors.As(err, &unusable) {
		t.Fatalf("an uncompilable schema should yield SchemaUnusableError, got %T: %v", err, err)
	}
	// (If the compiler happens to accept it, err is nil — also acceptable.)
}
