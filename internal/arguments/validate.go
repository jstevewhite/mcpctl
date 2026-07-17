package arguments

import (
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/jstevewhite/mcpctl/internal/apperror"
)

// SchemaUnusableError signals that a tool's input schema could not be compiled
// locally. The caller should warn and let the server validate (spec §2.5) — an
// unusable schema must never make a tool permanently unusable.
type SchemaUnusableError struct{ Err error }

func (e *SchemaUnusableError) Error() string {
	return "tool input schema could not be compiled: " + e.Err.Error()
}
func (e *SchemaUnusableError) Unwrap() error { return e.Err }

// Validate checks args against the tool's JSON Schema. A nil schema (or a schema
// that will not compile) does not fail the call: nil → nil; uncompilable →
// *SchemaUnusableError. A genuine schema violation → apperror KindInvalidArgs.
func Validate(schema any, args map[string]any) error {
	if schema == nil {
		return nil
	}
	if args == nil {
		args = map[string]any{} // validate a no-args call as an empty object
	}
	c := jsonschema.NewCompiler()
	const url = "mem://tool-input-schema"
	if err := c.AddResource(url, schema); err != nil {
		return &SchemaUnusableError{Err: err}
	}
	sch, err := c.Compile(url)
	if err != nil {
		return &SchemaUnusableError{Err: err}
	}
	if err := sch.Validate(any(args)); err != nil {
		return apperror.Wrap(apperror.KindInvalidArgs, err, "arguments do not match the tool's input schema")
	}
	return nil
}
