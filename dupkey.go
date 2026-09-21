// Copyright (c) 2026, Peter Ohler, All rights reserved.

package ojg

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// DupKeyPolicy determines how the parsers handle duplicate object keys.
type DupKeyPolicy int

const (
	// DupKeyLast keeps the last value of a duplicate object key. This is
	// the default and matches the historical behavior of the parsers.
	DupKeyLast DupKeyPolicy = iota
	// DupKeyFirst keeps the first value of a duplicate object key. Later
	// values are parsed but discarded.
	DupKeyFirst
	// DupKeyReject stops parsing with a *DupKeyError as soon as a duplicate
	// object key is detected, before the value of the duplicate key is
	// allocated.
	DupKeyReject
	// DupKeyReport keeps the last value (like DupKeyLast) but records a
	// DupKeyDiag for every duplicate object key encountered.
	DupKeyReport
)

const (
	// DefaultDupKeyMaxDiags is the default maximum number of retained
	// duplicate key diagnostics.
	DefaultDupKeyMaxDiags = 128
	// DefaultDupKeyMaxPath is the default maximum length of a retained
	// diagnostic path, in bytes.
	DefaultDupKeyMaxPath = 1024
	// DefaultDupKeyMaxSnippet is the default maximum length of a retained
	// raw input fragment, in bytes.
	DefaultDupKeyMaxSnippet = 64
)

// DupKeyDiag describes a single duplicate object key occurrence.
type DupKeyDiag struct {
	// Path is the JSON Pointer (RFC 6901) to the duplicate key, built from
	// decoded object keys and array indexes. It is truncated to the
	// configured maximum path length.
	Path string

	// Key is the decoded duplicate key. Keys that are spelled differently
	// in the raw input but decode to the same string (e.g. "a" and
	// "a") are considered duplicates.
	Key string

	// FirstOffset is the byte offset of the first occurrence of the key in
	// the raw UTF-8 input, pointing at the opening quote of the key token.
	FirstOffset int64

	// DupOffset is the byte offset of the duplicate occurrence of the key
	// in the raw UTF-8 input, pointing at the opening quote of the key
	// token.
	DupOffset int64

	// Snippet is a fragment of the raw input at the duplicate key token,
	// truncated to the configured maximum snippet length. When the key
	// token spans multiple reads only the trailing fragment is retained.
	Snippet string
}

// DupKeyOptions configures duplicate object key handling for the parsers in
// the gen and oj packages. The zero value selects DupKeyLast which matches
// the historical parser behavior.
type DupKeyOptions struct {
	// Policy selects the duplicate key handling mode.
	Policy DupKeyPolicy

	// MaxDiags caps the number of diagnostics retained in DupKeyReport
	// mode. Zero selects DefaultDupKeyMaxDiags and a negative value removes
	// the limit. The total number of detected duplicates is always counted,
	// even after the retained limit is reached.
	MaxDiags int

	// MaxPath caps the length in bytes of retained diagnostic paths. Zero
	// selects DefaultDupKeyMaxPath and a negative value removes the limit.
	MaxPath int

	// MaxSnippet caps the length in bytes of retained raw input
	// fragments. Zero selects DefaultDupKeyMaxSnippet and a negative value
	// removes the limit.
	MaxSnippet int

	// OnDup, when set, is called with every diagnostic in DupKeyReport
	// mode, including diagnostics beyond the retained MaxDiags limit.
	OnDup func(DupKeyDiag)
}

// DupKeyError is returned by the parsers when DupKeyReject is selected and a
// duplicate object key is encountered.
type DupKeyError struct {
	Diag DupKeyDiag
}

// Error returns a string representation of the error.
func (e *DupKeyError) Error() string {
	return fmt.Sprintf("duplicate object key %q at %s (offset %d, first occurrence at offset %d)",
		e.Diag.Key, e.Diag.Path, e.Diag.DupOffset, e.Diag.FirstOffset)
}

// DupTracker tracks object keys while parsing and applies a DupKeyPolicy. It
// is shared by the gen and oj parsers so both follow the same duplicate key
// contract. A DupTracker is driven by parser events; it is not safe for
// concurrent use.
type DupTracker struct {
	opts       DupKeyOptions
	dups       []DupKeyDiag
	total      int
	maxDiags   int
	maxPath    int
	maxSnippet int
	segs       []string // path segment of each open container, root first
	array      []bool   // true if the open container is an array
	keys       []string // last completed key of each open object
	idxs       []int    // next element index of each open array
	offs       []map[string]int64
	skip       []bool // pending keep-first skip of each open object
}

// Reset prepares the tracker for a new parse using the provided options. An
// invalid policy is treated as DupKeyLast.
func (t *DupTracker) Reset(opts DupKeyOptions) {
	if opts.Policy < DupKeyLast || DupKeyReport < opts.Policy {
		opts.Policy = DupKeyLast
	}
	t.opts = opts
	t.maxDiags = dupLimit(opts.MaxDiags, DefaultDupKeyMaxDiags)
	t.maxPath = dupLimit(opts.MaxPath, DefaultDupKeyMaxPath)
	t.maxSnippet = dupLimit(opts.MaxSnippet, DefaultDupKeyMaxSnippet)
	t.dups = t.dups[:0]
	t.total = 0
	t.segs = t.segs[:0]
	t.array = t.array[:0]
	t.keys = t.keys[:0]
	t.idxs = t.idxs[:0]
	t.offs = t.offs[:0]
	t.skip = t.skip[:0]
}

func dupLimit(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// Enabled returns true if duplicate key handling other than the default
// keep-last behavior is active.
func (t *DupTracker) Enabled() bool {
	return t.opts.Policy != DupKeyLast
}

// TrackDiag returns true if paths and diagnostics are tracked, which is the
// case for the DupKeyReject and DupKeyReport policies.
func (t *DupTracker) TrackDiag() bool {
	return t.opts.Policy == DupKeyReject || t.opts.Policy == DupKeyReport
}

// Open notes that a container (object or array) was opened. It must be called
// while the container is the innermost open container.
func (t *DupTracker) Open(array bool) {
	var seg string
	if n := len(t.array); 0 < n && t.TrackDiag() {
		if t.array[n-1] {
			seg = strconv.Itoa(t.idxs[n-1])
		} else {
			seg = t.keys[n-1]
		}
	}
	t.array = append(t.array, array)
	t.segs = append(t.segs, seg)
	t.keys = append(t.keys, "")
	t.idxs = append(t.idxs, 0)
	t.skip = append(t.skip, false)
	var m map[string]int64
	if !array {
		m = make(map[string]int64, 8)
	}
	t.offs = append(t.offs, m)
}

// Close notes that the innermost container was closed.
func (t *DupTracker) Close() {
	if n := len(t.array); 0 < n {
		t.array = t.array[:n-1]
		t.segs = t.segs[:n-1]
		t.keys = t.keys[:n-1]
		t.idxs = t.idxs[:n-1]
		t.offs[n-1] = nil
		t.offs = t.offs[:n-1]
		t.skip = t.skip[:n-1]
	}
}

// ValueAdded notes that a value was added to the innermost container. It only
// advances the element index of arrays.
func (t *DupTracker) ValueAdded() {
	if n := len(t.array); 0 < n && t.array[n-1] {
		t.idxs[n-1]++
	}
}

// KeyCompleted notes that an object key finished decoding. The key is the
// decoded key string, off is the byte offset of the opening quote of the key
// token in the raw UTF-8 input, and raw is the raw key token fragment used
// for diagnostic snippets. A *DupKeyError is returned when the policy is
// DupKeyReject and the key duplicates an earlier key in the same object.
func (t *DupTracker) KeyCompleted(key string, off int64, raw string) error {
	top := len(t.array) - 1
	if top < 0 || t.array[top] {
		return nil
	}
	if t.TrackDiag() {
		t.keys[top] = key
	}
	m := t.offs[top]
	if m == nil {
		return nil
	}
	first, dup := m[key]
	if !dup {
		m[key] = off
		return nil
	}
	switch t.opts.Policy {
	case DupKeyFirst:
		t.skip[top] = true
	case DupKeyReject, DupKeyReport:
		diag := t.buildDiag(key, first, off, raw)
		if t.opts.Policy == DupKeyReject {
			return &DupKeyError{Diag: diag}
		}
		t.total++
		if t.opts.OnDup != nil {
			t.opts.OnDup(diag)
		}
		if t.maxDiags < 0 || len(t.dups) < t.maxDiags {
			t.dups = append(t.dups, diag)
		}
	}
	return nil
}

// SkipAssign returns true once when the value of a duplicate key must be
// discarded, which only happens with the DupKeyFirst policy.
func (t *DupTracker) SkipAssign() bool {
	if top := len(t.array) - 1; 0 <= top && t.skip[top] {
		t.skip[top] = false
		return true
	}
	return false
}

// Diags returns the retained diagnostics and the total number of duplicates
// detected during the current parse.
func (t *DupTracker) Diags() ([]DupKeyDiag, int) {
	return t.dups, t.total
}

func (t *DupTracker) buildDiag(key string, first, dup int64, raw string) DupKeyDiag {
	var b strings.Builder
	for _, seg := range t.segs[1:] {
		b.WriteByte('/')
		appendJSONPointer(&b, seg)
	}
	b.WriteByte('/')
	appendJSONPointer(&b, key)
	return DupKeyDiag{
		Path:        truncateDupString(b.String(), t.maxPath),
		Key:         key,
		FirstOffset: first,
		DupOffset:   dup,
		Snippet:     truncateDupString(raw, t.maxSnippet),
	}
}

// appendJSONPointer appends s to b escaped according to RFC 6901.
func appendJSONPointer(b *strings.Builder, s string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '~':
			b.WriteString("~0")
		case '/':
			b.WriteString("~1")
		default:
			b.WriteByte(s[i])
		}
	}
}

// truncateDupString truncates s to max bytes on a rune boundary, appending an
// ellipsis when truncated. A negative max removes the limit.
func truncateDupString(s string, max int) string {
	if max < 0 || len(s) <= max {
		return s
	}
	if 3 < max {
		cut := max - 3
		for 0 < cut && !utf8.RuneStart(s[cut]) {
			cut--
		}
		return s[:cut] + "..."
	}
	cut := max
	for 0 < cut && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
