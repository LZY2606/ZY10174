// Copyright (c) 2026, Peter Ohler, All rights reserved.

package oj

import "github.com/ohler55/ojg/gen"

// Duplicate object key handling is shared between the gen and oj parsers.
// The types and constants below are aliases for the gen package definitions
// so that both parsers expose the exact same options and diagnostics.

// DupKeyMode determines how a parser handles duplicate object keys.
type DupKeyMode = gen.DupKeyMode

const (
	// DupKeyLast keeps the last value associated with a duplicate key. This
	// is the default and matches the historical behavior of the parsers.
	DupKeyLast = gen.DupKeyLast
	// DupKeyFirst keeps the first value associated with a duplicate key.
	DupKeyFirst = gen.DupKeyFirst
	// DupKeyReject stops parsing with a *DupKeyError on a duplicate key.
	DupKeyReject = gen.DupKeyReject
	// DupKeyReport records a DupKeyDiag for each duplicate key and keeps
	// parsing.
	DupKeyReport = gen.DupKeyReport
)

// DupKeyDiag describes a single duplicate object key occurrence.
type DupKeyDiag = gen.DupKeyDiag

// DupKeyError is returned when DupKeyReject mode is enabled and a duplicate
// object key is encountered.
type DupKeyError = gen.DupKeyError

// DupKeyOptions configures duplicate object key handling for the parser.
type DupKeyOptions = gen.DupKeyOptions
