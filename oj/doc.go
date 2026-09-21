// // Copyright (c) 2020, Peter Ohler, All rights reserved.

// Package oj contains functions and types to support building simple types
// where simple types are:
//
//	nil
//	bool
//	int64
//	float64
//	string
//	time.Time
//	[]any
//	map[string]any
//
// # Parser
//
// Parse a JSON file or stream. The parser can be used on a single JSON document or a
// document with multiple JSON elements. An option also exists to allow comments
// in the JSON.
//
//	v, err := oj.ParseString("[true,[false,[null],123],456]")
//
// or for a performance gain on repeated parses:
//
//	var p oj.Parser
//	v, err := p.Parse([]byte("[true,[false,[null],123],456]"))
//
// # Duplicate Object Keys
//
// Duplicate object keys are handled according to DupKeyOptions, a type
// shared with the gen package so both parsers expose identical options and
// diagnostics. The option can be set on a Parser or passed as a parse
// argument.
//
//	v, err := oj.ParseString(`{"a":1,"a":2}`, &oj.DupKeyOptions{Mode: oj.DupKeyReport})
//
// The default mode keeps the last value, matching the historical behavior.
// Other modes keep the first value, reject the input with a *DupKeyError
// before the duplicate value is parsed, or collect DupKeyDiag diagnostics
// while continuing. Diagnostics report a JSON Pointer path, the decoded key,
// and byte offsets into the original UTF-8 input. Keys are compared after
// decoding, so escaped spellings of the same key are duplicates. Report mode
// bounds memory with configurable limits on the number of diagnostics, the
// path length, and the retained input snippet length. Tracking adds one map
// lookup per object key; the default mode adds no overhead.
//
// # Validator
//
// Validates a JSON file or stream. It can be used on a single JSON document or a
// document with multiple JSON elements. An option also exists to allow comments
// in the JSON.
//
//	err := oj.ValidateString("[true,[false,[null],123],456]")
//
// or for a slight performance gain on repeated validations:
//
//	var v oj.Validator
//	err := v.Validate([]byte("[true,[false,[null],123],456]"))
//
// # Builder
//
// An example of building simple data is:
//
//	var b oj.Builder
//
//	b.Object()
//	b.Value(1, "a")
//	b.Array("b")
//	b.Value(2)
//	b.Pop()
//	b.Pop()
//	v := b.Result()
//
//	// v: map[string]any{"a": 1, "b": []any{2}}
//
// # Writer
//
// The writer function's output data values to JSON. The basic oj.JSON() attempts
// to build JSON from any data provided skipping types that can not be converted.
//
//	s := oj.JSON([]any{1, 2, "abc", true})
//
// Output can also be use with an io.Writer.
//
//	var b strings.Builder
//
//	err := oj.Write(&b, []any{1, 2, "abc", true})
package oj
