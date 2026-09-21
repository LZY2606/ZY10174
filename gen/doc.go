// Copyright (c) 2020, Peter Ohler, All rights reserved.

// Package gen provides type safe generic types. They are type safe in that array
// and objects can only be constructed of other types in the package. The basic
// types are:
//
//	Bool
//	Int
//	Float
//	String
//	Time
//
// The collection types are Array and Object. All the types implement the Node
// interface which is relatively simple interface defined primarily to restrict
// what can be in the collection types. The Node interface should not be used to
// define new generic types.
//
// Also included in the package are a builder and parser that behave like the
// parser and builder in the oj package except for gen types.
//
// # Duplicate Object Keys
//
// The Parser handles duplicate object keys according to DupKeyOptions, which
// is shared with the oj package. By default the last value wins, matching
// the historical behavior. The mode can instead keep the first value, reject
// the input with a *DupKeyError before the duplicate value is parsed, or
// collect DupKeyDiag diagnostics (JSON Pointer path, decoded key, and raw
// input byte offsets) while continuing to parse.
package gen
