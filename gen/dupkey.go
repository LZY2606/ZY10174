// Copyright (c) 2026, Peter Ohler, All rights reserved.

package gen

import (
	"fmt"
	"strings"
)

// DupKeyMode determines how a parser handles duplicate object keys.
type DupKeyMode int

const (
	// DupKeyLast keeps the last value associated with a duplicate key. This
	// is the default and matches the historical behavior of the parsers.
	DupKeyLast DupKeyMode = iota
	// DupKeyFirst keeps the first value associated with a duplicate key and
	// silently discards the values of later occurrences.
	DupKeyFirst
	// DupKeyReject stops parsing and returns a *DupKeyError as soon as a
	// duplicate key is detected, before the value of the duplicate key is
	// parsed or allocated.
	DupKeyReject
	// DupKeyReport records a DupKeyDiag for each duplicate key and keeps
	// parsing. The last value wins, matching DupKeyLast semantics.
	DupKeyReport
)

const (
	// DefaultDupKeyMaxDiags is the default limit on the number of
	// diagnostics collected in DupKeyReport mode.
	DefaultDupKeyMaxDiags = 64
	// DefaultDupKeyMaxPath is the default limit in bytes on the length of
	// the Path member of a DupKeyDiag.
	DefaultDupKeyMaxPath = 256
	// DefaultDupKeyMaxSnippet is the default limit in bytes on the length
	// of the Snippet member of a DupKeyDiag.
	DefaultDupKeyMaxSnippet = 64
)

// DupKeyDiag describes a single duplicate object key occurrence.
type DupKeyDiag struct {
	// Path is a JSON Pointer (RFC 6901) to the duplicate member, e.g.
	// "/a/0/b". It is truncated to MaxPath bytes if longer.
	Path string
	// Key is the decoded key string. Keys that differ in the raw input but
	// decode to the same string (e.g. "a" and "\u0061") are duplicates.
	Key string
	// FirstOffset is the byte offset in the original UTF-8 input of the
	// opening quote of the first occurrence of the key.
	FirstOffset int64
	// DupOffset is the byte offset in the original UTF-8 input of the
	// opening quote of the duplicate occurrence of the key.
	DupOffset int64
	// Snippet is a fragment of the raw input ending at the duplicate key,
	// limited to MaxSnippet bytes. It is only populated in DupKeyReport
	// mode.
	Snippet string
}

// DupKeyError is returned by parsers when DupKeyReject mode is enabled and a
// duplicate object key is encountered.
type DupKeyError struct {
	Diag DupKeyDiag
}

// Error returns a description of the duplicate key error.
func (e *DupKeyError) Error() string {
	return fmt.Sprintf("duplicate object key %q at %s (first at byte offset %d, again at %d)",
		e.Diag.Key, e.Diag.Path, e.Diag.FirstOffset, e.Diag.DupOffset)
}

// DupKeyOptions configures duplicate object key handling for the parsers in
// the gen and oj packages. The zero value keeps the historical behavior of
// keeping the last value for a duplicate key.
//
// Diagnostics collected in DupKeyReport mode are appended to Diags which is
// reset at the start of each Parse or ParseReader call.
type DupKeyOptions struct {
	// Mode selects the duplicate key behavior. See DupKeyMode.
	Mode DupKeyMode
	// MaxDiags limits the number of diagnostics kept in DupKeyReport
	// mode. Values less than one select DefaultDupKeyMaxDiags.
	MaxDiags int
	// MaxPath limits the byte length of the Path member of each
	// diagnostic. Values less than one select DefaultDupKeyMaxPath.
	MaxPath int
	// MaxSnippet limits the byte length of the Snippet member of each
	// diagnostic. Values less than one select DefaultDupKeyMaxSnippet.
	MaxSnippet int
	// Diags holds the diagnostics collected in DupKeyReport mode.
	Diags []DupKeyDiag
}

func (o *DupKeyOptions) maxDiags() int {
	if 0 < o.MaxDiags {
		return o.MaxDiags
	}
	return DefaultDupKeyMaxDiags
}

func (o *DupKeyOptions) maxPath() int {
	if 0 < o.MaxPath {
		return o.MaxPath
	}
	return DefaultDupKeyMaxPath
}

func (o *DupKeyOptions) maxSnippet() int {
	if 0 < o.MaxSnippet {
		return o.MaxSnippet
	}
	return DefaultDupKeyMaxSnippet
}

var dupSegEscaper = strings.NewReplacer("~", "~0", "/", "~1")

// DupKeyTracker tracks the parser state needed to detect duplicate object
// keys. It is shared by the gen and oj parsers so that both report identical
// diagnostics. A nil Opts disables tracking entirely.
type DupKeyTracker struct {
	// Opts is the active configuration or nil when tracking is disabled.
	Opts *DupKeyOptions

	segs    []string
	firsts  []map[string]int64
	hist    []byte
	histPos int
	foff    int64
	keyOff  int64
}

// Reset prepares the tracker for a new parse. A nil opts or a mode of
// DupKeyLast or DupKeyFirst disables tracking; DupKeyFirst is handled by the
// parsers without tracking state.
func (t *DupKeyTracker) Reset(opts *DupKeyOptions) {
	t.Opts = nil
	t.segs = t.segs[:0]
	t.firsts = t.firsts[:0]
	t.hist = t.hist[:0]
	t.histPos = 0
	t.foff = 0
	t.keyOff = 0
	if opts != nil && (opts.Mode == DupKeyReject || opts.Mode == DupKeyReport) {
		t.Opts = opts
		opts.Diags = opts.Diags[:0]
	}
}

// Active returns true if duplicate keys are being tracked.
func (t *DupKeyTracker) Active() bool {
	return t.Opts != nil
}

// SetBase sets the absolute input offset of the next buffer, used to account
// for a skipped BOM.
func (t *DupKeyTracker) SetBase(off int64) {
	t.foff = off
}

// KeyStart records the offset in the current buffer of the opening quote of
// a key.
func (t *DupKeyTracker) KeyStart(off int) {
	t.keyOff = t.foff + int64(off)
}

// OpenObject notes the start of an object. The seg argument is the JSON
// Pointer segment of the object within its parent.
func (t *DupKeyTracker) OpenObject(seg string) {
	t.segs = append(t.segs, seg)
	t.firsts = append(t.firsts, make(map[string]int64, mapInitSize))
}

// OpenArray notes the start of an array.
func (t *DupKeyTracker) OpenArray(seg string) {
	t.segs = append(t.segs, seg)
}

// CloseObject notes the end of the innermost object.
func (t *DupKeyTracker) CloseObject() {
	t.segs = t.segs[:len(t.segs)-1]
	t.firsts[len(t.firsts)-1] = nil
	t.firsts = t.firsts[:len(t.firsts)-1]
}

// CloseArray notes the end of the innermost array.
func (t *DupKeyTracker) CloseArray() {
	t.segs = t.segs[:len(t.segs)-1]
}

// CheckKey checks a completed key against the keys already seen in the
// innermost object. The off argument is the offset in buf of the closing
// quote of the key. A *DupKeyError is returned in DupKeyReject mode.
func (t *DupKeyTracker) CheckKey(key string, buf []byte, off int) error {
	firsts := t.firsts[len(t.firsts)-1]
	firstOff, dup := firsts[key]
	if !dup {
		firsts[key] = t.keyOff
		return nil
	}
	d := DupKeyDiag{
		Key:         key,
		FirstOffset: firstOff,
		DupOffset:   t.keyOff,
	}
	if t.Opts.Mode == DupKeyReject {
		d.Path = t.path(key)
		return &DupKeyError{Diag: d}
	}
	if len(t.Opts.Diags) < t.Opts.maxDiags() {
		d.Path = t.path(key)
		d.Snippet = t.snippet(buf, off)
		t.Opts.Diags = append(t.Opts.Diags, d)
	}
	return nil
}

// EndBuffer is called after each buffer is consumed. It advances the input
// offset and, in DupKeyReport mode, retains a bounded fragment of the raw
// input for diagnostic snippets.
func (t *DupKeyTracker) EndBuffer(buf []byte) {
	if t.Opts == nil {
		return
	}
	if t.Opts.Mode == DupKeyReport && t.histPos < len(buf) {
		t.hist = append(t.hist, buf[t.histPos:]...)
		if max := t.Opts.maxSnippet(); max < len(t.hist) {
			t.hist = append(t.hist[:0], t.hist[len(t.hist)-max:]...)
		}
	}
	t.histPos = 0
	t.foff += int64(len(buf))
}

func (t *DupKeyTracker) snippet(buf []byte, off int) string {
	t.hist = append(t.hist, buf[t.histPos:off+1]...)
	t.histPos = off + 1
	if max := t.Opts.maxSnippet(); max < len(t.hist) {
		t.hist = append(t.hist[:0], t.hist[len(t.hist)-max:]...)
	}
	return string(t.hist)
}

func (t *DupKeyTracker) path(key string) string {
	max := t.Opts.maxPath()
	var sb strings.Builder
	n := 0
	write := func(s string) {
		if n < max {
			if rem := max - n; rem < len(s) {
				s = s[:rem]
			}
			sb.WriteString(s)
			n += len(s)
		}
	}
	for i, s := range t.segs {
		if i == 0 {
			continue // the root container has no segment
		}
		write("/")
		write(s)
	}
	write("/")
	write(dupSegEscaper.Replace(key))
	return sb.String()
}

// EscapeDupSeg escapes a JSON Pointer path segment according to RFC 6901.
func EscapeDupSeg(s string) string {
	return dupSegEscaper.Replace(s)
}
