// Copyright (c) 2026, Peter Ohler, All rights reserved.

package oj_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ohler55/ojg/gen"
	"github.com/ohler55/ojg/oj"
	"github.com/ohler55/ojg/tt"
)

// dupChunkReader returns the content in fixed size chunks to simulate a
// stream that splits tokens, including in the middle of an escape sequence.
type dupChunkReader struct {
	content []byte
	chunk   int
	pos     int
}

func (r *dupChunkReader) Read(p []byte) (int, error) {
	if len(r.content) <= r.pos {
		return 0, io.EOF
	}
	n := r.chunk
	if remain := len(r.content) - r.pos; remain < n {
		n = remain
	}
	if len(p) < n {
		n = len(p)
	}
	copy(p, r.content[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}

func TestDupKeyOjDefaultKeepsLast(t *testing.T) {
	v, err := oj.Parse([]byte(`{"a":1,"a":2}`))
	tt.Nil(t, err)
	tt.Equal(t, 2, v.(map[string]any)["a"], "default mode should keep the last value")
}

func TestDupKeyOjSharedTypes(t *testing.T) {
	// The oj and gen packages share the same option and diagnostic types.
	var opts *oj.DupKeyOptions = &gen.DupKeyOptions{Mode: oj.DupKeyReport}
	v, err := oj.Parse([]byte(`{"a":1,"a":2}`), opts)
	tt.Nil(t, err)
	tt.Equal(t, 2, v.(map[string]any)["a"], "report mode should keep the last value")
	tt.Equal(t, 1, len(opts.Diags), "diag count")
	var d oj.DupKeyDiag = opts.Diags[0]
	tt.Equal(t, "/a", d.Path, "diag path")
	tt.Equal(t, "a", d.Key, "diag key")
	tt.Equal(t, int64(1), d.FirstOffset, "diag first offset")
	tt.Equal(t, int64(7), d.DupOffset, "diag dup offset")
}

func TestDupKeyOjFirst(t *testing.T) {
	var p oj.Parser
	p.DupKey = &oj.DupKeyOptions{Mode: oj.DupKeyFirst}
	v, err := p.Parse([]byte(`{"a":1,"a":2,"b":{"c":3,"c":4}}`))
	tt.Nil(t, err)
	obj := v.(map[string]any)
	tt.Equal(t, 1, obj["a"], "first mode should keep the first value")
	tt.Equal(t, 3, obj["b"].(map[string]any)["c"], "first mode should apply to nested objects")
}

func TestDupKeyOjReject(t *testing.T) {
	var p oj.Parser
	src := `{"a":[{"x":1,"x":2}]}`
	_, err := p.Parse([]byte(src), &oj.DupKeyOptions{Mode: oj.DupKeyReject})
	tt.NotNil(t, err)
	var derr *oj.DupKeyError
	if !errors.As(err, &derr) {
		t.Fatalf("expected a *oj.DupKeyError, got %T: %s", err, err)
	}
	var _ *gen.DupKeyError = derr // shared type with the gen package
	tt.Equal(t, "/a/0/x", derr.Diag.Path, "reject diag path")
	tt.Equal(t, "x", derr.Diag.Key, "reject diag key")
	tt.Equal(t, int64(strings.Index(src, `"x"`)), derr.Diag.FirstOffset, "reject first offset")
	tt.Equal(t, int64(strings.LastIndex(src, `"x"`)), derr.Diag.DupOffset, "reject dup offset")
}

func TestDupKeyOjRejectStopsBeforeValue(t *testing.T) {
	var p oj.Parser
	// The duplicate key is followed by an unterminated array. A reject error
	// proves parsing stopped before the value was parsed or allocated.
	_, err := p.Parse([]byte(`{"a":1,"a":[1,2,3`), &oj.DupKeyOptions{Mode: oj.DupKeyReject})
	var derr *oj.DupKeyError
	if !errors.As(err, &derr) {
		t.Fatalf("expected a *oj.DupKeyError before the value is parsed, got %v", err)
	}
}

func TestDupKeyOjReportReaderChunked(t *testing.T) {
	// Small chunk sizes split the é key and escapes across reads.
	src := `{"é":1,"é":2}`
	for _, chunk := range []int{1, 3, 4096} {
		var p oj.Parser
		opts := &oj.DupKeyOptions{Mode: oj.DupKeyReport}
		v, err := p.ParseReader(&dupChunkReader{content: []byte(src), chunk: chunk}, opts)
		tt.Nil(t, err, "chunk size %d", chunk)
		tt.Equal(t, 2, v.(map[string]any)["é"], "chunk size %d", chunk)
		tt.Equal(t, 1, len(opts.Diags), "chunk size %d", chunk)
		d := opts.Diags[0]
		tt.Equal(t, "/é", d.Path, "chunk size %d", chunk)
		tt.Equal(t, "é", d.Key, "chunk size %d", chunk)
		tt.Equal(t, int64(1), d.FirstOffset, "chunk size %d: byte offset, not rune index", chunk)
		tt.Equal(t, int64(8), d.DupOffset, "chunk size %d: byte offset, not rune index", chunk)
	}
}

func TestDupKeyOjReportLimits(t *testing.T) {
	var p oj.Parser
	src := `{"a":1,"a":2,"a":3,"a":4}`
	opts := &oj.DupKeyOptions{Mode: oj.DupKeyReport, MaxDiags: 1, MaxPath: 4, MaxSnippet: 5}
	_, err := p.Parse([]byte(src), opts)
	tt.Nil(t, err)
	tt.Equal(t, 1, len(opts.Diags), "diagnostics should be capped by MaxDiags")
	d := opts.Diags[0]
	if len(d.Path) > 4 {
		t.Fatalf("path %q exceeds MaxPath", d.Path)
	}
	if len(d.Snippet) > 5 {
		t.Fatalf("snippet %q exceeds MaxSnippet", d.Snippet)
	}
}
