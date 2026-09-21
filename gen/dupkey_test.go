// Copyright (c) 2026, Peter Ohler, All rights reserved.

package gen_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ohler55/ojg/gen"
	"github.com/ohler55/ojg/tt"
)

// chunkReader returns the content in fixed size chunks to simulate a stream
// that splits tokens, including in the middle of an escape sequence.
type chunkReader struct {
	content []byte
	chunk   int
	pos     int
}

func (r *chunkReader) Read(p []byte) (int, error) {
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

func TestDupKeyDefaultKeepsLast(t *testing.T) {
	var p gen.Parser
	v, err := p.Parse([]byte(`{"a":1,"a":2}`))
	tt.Nil(t, err)
	tt.Equal(t, 2, v.(gen.Object)["a"], "default mode should keep the last value")
}

func TestDupKeyFirst(t *testing.T) {
	var p gen.Parser
	opts := &gen.DupKeyOptions{Mode: gen.DupKeyFirst}
	v, err := p.Parse([]byte(`{"a":1,"a":2,"b":{"c":3,"c":4}}`), opts)
	tt.Nil(t, err)
	obj := v.(gen.Object)
	tt.Equal(t, 1, obj["a"], "first mode should keep the first value")
	tt.Equal(t, 3, obj["b"].(gen.Object)["c"], "first mode should apply to nested objects")
	tt.Equal(t, 0, len(opts.Diags), "first mode should not record diagnostics")
}

func TestDupKeyReject(t *testing.T) {
	var p gen.Parser
	src := `{"a":{"b":1,"b":2}}`
	_, err := p.Parse([]byte(src), &gen.DupKeyOptions{Mode: gen.DupKeyReject})
	tt.NotNil(t, err)
	var derr *gen.DupKeyError
	if !errors.As(err, &derr) {
		t.Fatalf("expected a *gen.DupKeyError, got %T: %s", err, err)
	}
	d := derr.Diag
	tt.Equal(t, "/a/b", d.Path, "reject diag path")
	tt.Equal(t, "b", d.Key, "reject diag key")
	tt.Equal(t, int64(strings.Index(src, `"b"`)), d.FirstOffset, "reject diag first offset")
	tt.Equal(t, int64(strings.LastIndex(src, `"b"`)), d.DupOffset, "reject diag dup offset")
}

func TestDupKeyRejectStopsBeforeValue(t *testing.T) {
	var p gen.Parser
	// The duplicate key is followed by an unterminated array. A reject error
	// proves parsing stopped before the value was parsed or allocated.
	_, err := p.Parse([]byte(`{"a":1,"a":[1,2,3`), &gen.DupKeyOptions{Mode: gen.DupKeyReject})
	var derr *gen.DupKeyError
	if !errors.As(err, &derr) {
		t.Fatalf("expected a *gen.DupKeyError before the value is parsed, got %v", err)
	}
}

func TestDupKeyReport(t *testing.T) {
	var p gen.Parser
	src := `{"a":1,"a":2,"a":3}`
	opts := &gen.DupKeyOptions{Mode: gen.DupKeyReport}
	v, err := p.Parse([]byte(src), opts)
	tt.Nil(t, err)
	tt.Equal(t, 3, v.(gen.Object)["a"], "report mode should keep the last value")
	tt.Equal(t, 2, len(opts.Diags), "report mode should record each duplicate")
	first := int64(strings.Index(src, `"a"`))
	for i, d := range opts.Diags {
		tt.Equal(t, "/a", d.Path, "diag %d path", i)
		tt.Equal(t, "a", d.Key, "diag %d key", i)
		tt.Equal(t, first, d.FirstOffset, "diag %d first offset", i)
		if !strings.HasSuffix(d.Snippet, `"a"`) {
			t.Fatalf("diag %d snippet %q should end with the duplicate key", i, d.Snippet)
		}
	}
	tt.Equal(t, int64(7), opts.Diags[0].DupOffset, "diag 0 dup offset")
	tt.Equal(t, int64(13), opts.Diags[1].DupOffset, "diag 1 dup offset")
}

func TestDupKeyNestedAndArrayPaths(t *testing.T) {
	var p gen.Parser
	src := `{"a":[{"x":1,"x":2},{"y":{"x":3,"x":4}}]}`
	opts := &gen.DupKeyOptions{Mode: gen.DupKeyReport}
	_, err := p.Parse([]byte(src), opts)
	tt.Nil(t, err)
	tt.Equal(t, 2, len(opts.Diags), "diag count")
	tt.Equal(t, "/a/0/x", opts.Diags[0].Path, "path of duplicate in array member")
	tt.Equal(t, "/a/1/y/x", opts.Diags[1].Path, "path of duplicate in nested object")
}

func TestDupKeyEscapedKeys(t *testing.T) {
	var p gen.Parser
	// Both keys decode to "a" even though the raw spellings differ.
	src := `{"\u0061":1,"a":2}`
	opts := &gen.DupKeyOptions{Mode: gen.DupKeyReport}
	_, err := p.Parse([]byte(src), opts)
	tt.Nil(t, err)
	tt.Equal(t, 1, len(opts.Diags), "escaped and plain keys that decode alike are duplicates")
	tt.Equal(t, "a", opts.Diags[0].Key, "diag key should be the decoded string")
	tt.Equal(t, int64(1), opts.Diags[0].FirstOffset, "first offset")
	tt.Equal(t, int64(12), opts.Diags[0].DupOffset, "dup offset")
}

func TestDupKeyEmptyKey(t *testing.T) {
	var p gen.Parser
	opts := &gen.DupKeyOptions{Mode: gen.DupKeyReport}
	_, err := p.Parse([]byte(`{"":1,"":2}`), opts)
	tt.Nil(t, err)
	tt.Equal(t, 1, len(opts.Diags), "empty keys can be duplicates")
	tt.Equal(t, "", opts.Diags[0].Key, "diag key should be empty")
	tt.Equal(t, "/", opts.Diags[0].Path, "JSON Pointer for an empty key")
}

func TestDupKeyDifferentScopes(t *testing.T) {
	var p gen.Parser
	opts := &gen.DupKeyOptions{Mode: gen.DupKeyReport}
	_, err := p.Parse([]byte(`{"a":{"a":1},"b":{"a":2},"c":[{"a":3}]}`), opts)
	tt.Nil(t, err)
	tt.Equal(t, 0, len(opts.Diags), "same key in different objects is not a conflict")
}

func TestDupKeyByteOffsets(t *testing.T) {
	var p gen.Parser
	// é is two bytes in UTF-8. Offsets must be byte offsets, not rune indexes.
	src := `{"é":1,"é":2}`
	opts := &gen.DupKeyOptions{Mode: gen.DupKeyReport}
	_, err := p.Parse([]byte(src), opts)
	tt.Nil(t, err)
	tt.Equal(t, 1, len(opts.Diags), "diag count")
	tt.Equal(t, int64(1), opts.Diags[0].FirstOffset, "first offset is a byte offset")
	tt.Equal(t, int64(8), opts.Diags[0].DupOffset, "dup offset is a byte offset, not a rune index")
}

func TestDupKeyReaderChunked(t *testing.T) {
	// Small chunk sizes split the A escape sequence across reads.
	src := `{"\u0041":1,"A":2}`
	for _, chunk := range []int{1, 2, 3, 5, 4096} {
		var p gen.Parser
		opts := &gen.DupKeyOptions{Mode: gen.DupKeyReport}
		v, err := p.ParseReader(&chunkReader{content: []byte(src), chunk: chunk}, opts)
		tt.Nil(t, err, "chunk size %d", chunk)
		tt.Equal(t, 2, v.(gen.Object)["A"], "chunk size %d", chunk)
		tt.Equal(t, 1, len(opts.Diags), "chunk size %d", chunk)
		d := opts.Diags[0]
		tt.Equal(t, "A", d.Key, "chunk size %d", chunk)
		tt.Equal(t, int64(1), d.FirstOffset, "chunk size %d", chunk)
		tt.Equal(t, int64(12), d.DupOffset, "chunk size %d", chunk)
	}
}

func TestDupKeyRejectReader(t *testing.T) {
	var p gen.Parser
	src := `{"a":1,"b":{"c":2,"c":3}}`
	_, err := p.ParseReader(&chunkReader{content: []byte(src), chunk: 2},
		&gen.DupKeyOptions{Mode: gen.DupKeyReject})
	var derr *gen.DupKeyError
	if !errors.As(err, &derr) {
		t.Fatalf("expected a *gen.DupKeyError, got %v", err)
	}
	tt.Equal(t, "/b/c", derr.Diag.Path, "reject path from a reader")
	tt.Equal(t, int64(strings.LastIndex(src, `"c"`)), derr.Diag.DupOffset, "reject dup offset from a reader")
}

func TestDupKeyReportLimits(t *testing.T) {
	var p gen.Parser
	src := `{"a":1,"a":2,"a":3,"a":4,"a":5}`
	opts := &gen.DupKeyOptions{Mode: gen.DupKeyReport, MaxDiags: 2, MaxSnippet: 6}
	_, err := p.Parse([]byte(src), opts)
	tt.Nil(t, err)
	tt.Equal(t, 2, len(opts.Diags), "diagnostics should be capped by MaxDiags")
	for i, d := range opts.Diags {
		if len(d.Snippet) > 6 {
			t.Fatalf("diag %d snippet %q exceeds MaxSnippet", i, d.Snippet)
		}
	}
}

func TestDupKeyReportPathLimit(t *testing.T) {
	var p gen.Parser
	src := `{"aaaaaaaaaa":{"bbbbbbbbbb":{"cccccccccc":1,"cccccccccc":2}}}`
	opts := &gen.DupKeyOptions{Mode: gen.DupKeyReport, MaxPath: 12}
	_, err := p.Parse([]byte(src), opts)
	tt.Nil(t, err)
	tt.Equal(t, 1, len(opts.Diags), "diag count")
	if len(opts.Diags[0].Path) > 12 {
		t.Fatalf("path %q exceeds MaxPath", opts.Diags[0].Path)
	}
}

func TestDupKeyBomOffsets(t *testing.T) {
	var p gen.Parser
	src := "\xef\xbb\xbf" + `{"a":1,"a":2}`
	opts := &gen.DupKeyOptions{Mode: gen.DupKeyReport}
	_, err := p.Parse([]byte(src), opts)
	tt.Nil(t, err)
	tt.Equal(t, 1, len(opts.Diags), "diag count")
	tt.Equal(t, int64(4), opts.Diags[0].FirstOffset, "first offset includes the BOM")
	tt.Equal(t, int64(10), opts.Diags[0].DupOffset, "dup offset includes the BOM")
}
