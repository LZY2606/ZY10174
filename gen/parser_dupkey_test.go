// Copyright (c) 2026, Peter Ohler, All rights reserved.

package gen_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ohler55/ojg"
	"github.com/ohler55/ojg/gen"
	"github.com/ohler55/ojg/tt"
)

// chunkReader returns the content in chunks of size n to exercise the
// incremental parsing paths of ParseReader.
type chunkReader struct {
	content []byte
	n       int
	pos     int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.content) <= r.pos {
		return 0, io.EOF
	}
	cnt := r.n
	if remain := len(r.content) - r.pos; remain < cnt {
		cnt = remain
	}
	if len(p) < cnt {
		cnt = len(p)
	}
	copy(p, r.content[r.pos:r.pos+cnt])
	r.pos += cnt
	return cnt, nil
}

func TestParseDupKeyDefaultKeepsLast(t *testing.T) {
	var p gen.Parser
	v, err := p.Parse([]byte(`{"a":1,"a":2}`))
	tt.Nil(t, err)
	tt.Equal(t, gen.Object{"a": gen.Int(2)}, v, "default policy keeps the last value")
	_, total := p.DupKeyDiags()
	tt.Equal(t, 0, total, "default policy records no diagnostics")
}

func TestParseDupKeyFirst(t *testing.T) {
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyFirst
	v, err := p.Parse([]byte(`{"a":1,"a":2,"b":{"c":1,"c":2}}`))
	tt.Nil(t, err)
	tt.Equal(t, gen.Object{"a": gen.Int(1), "b": gen.Object{"c": gen.Int(1)}}, v,
		"keep-first keeps the first value at every level")
}

func TestParseDupKeyFirstNestedValue(t *testing.T) {
	// The value of a duplicate key can itself be a container. The skip
	// must apply to the duplicate key only, not to keys inside its value.
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyFirst
	v, err := p.Parse([]byte(`{"a":1,"a":{"b":2},"c":[3],"c":[4]}`))
	tt.Nil(t, err)
	tt.Equal(t, gen.Object{"a": gen.Int(1), "c": gen.Array{gen.Int(3)}}, v,
		"keep-first discards the whole duplicate value without disturbing it")
}

func TestParseDupKeyFirstNestedDupInDupValue(t *testing.T) {
	// A duplicate key inside the discarded value of another duplicate key
	// must not consume the outer skip.
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyFirst
	v, err := p.Parse([]byte(`{"a":1,"a":{"b":1,"b":2}}`))
	tt.Nil(t, err)
	tt.Equal(t, gen.Object{"a": gen.Int(1)}, v,
		"inner duplicate must not consume the outer keep-first skip")
}

func TestParseDupKeyFirstReader(t *testing.T) {
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyFirst
	v, err := p.ParseReader(&chunkReader{content: []byte(`{"a":1,"a":2}`), n: 3})
	tt.Nil(t, err)
	tt.Equal(t, gen.Object{"a": gen.Int(1)}, v)
}

func TestParseDupKeyReject(t *testing.T) {
	src := `{"a":1,"a":2}`
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyReject
	_, err := p.Parse([]byte(src))
	tt.NotNil(t, err)
	var de *ojg.DupKeyError
	tt.Equal(t, true, errors.As(err, &de), "error must be a *ojg.DupKeyError, got %T", err)
	tt.Equal(t, "/a", de.Diag.Path)
	tt.Equal(t, "a", de.Diag.Key)
	tt.Equal(t, int64(strings.Index(src, `"a"`)), de.Diag.FirstOffset)
	tt.Equal(t, int64(strings.LastIndex(src, `"a"`)), de.Diag.DupOffset)
}

func TestParseDupKeyRejectStopsBeforeValue(t *testing.T) {
	// The value of the duplicate key is invalid JSON. A reject before the
	// value is allocated must report the duplicate key, not the syntax
	// error.
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyReject
	_, err := p.Parse([]byte(`{"a":1,"a":!}`))
	tt.NotNil(t, err)
	var de *ojg.DupKeyError
	tt.Equal(t, true, errors.As(err, &de),
		"reject must stop at the duplicate key, before the value is parsed: %v", err)
}

func TestParseDupKeyReport(t *testing.T) {
	src := `{"a":1,"a":2,"a":3}`
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyReport
	v, err := p.Parse([]byte(src))
	tt.Nil(t, err)
	tt.Equal(t, gen.Object{"a": gen.Int(3)}, v, "report keeps the last value")
	dups, total := p.DupKeyDiags()
	tt.Equal(t, 2, total, "consecutive duplicates each produce a diagnostic")
	tt.Equal(t, 2, len(dups))
	first := int64(strings.Index(src, `"a"`))
	tt.Equal(t, first, dups[0].FirstOffset, "first offset of consecutive duplicates")
	tt.Equal(t, first, dups[1].FirstOffset, "first offset of consecutive duplicates")
	tt.Equal(t, int64(strings.Index(src, `"a":2`)), dups[0].DupOffset)
	tt.Equal(t, int64(strings.Index(src, `"a":3`)), dups[1].DupOffset)
	tt.Equal(t, `"a"`, dups[0].Snippet)
}

func TestParseDupKeyNested(t *testing.T) {
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyReport
	_, err := p.Parse([]byte(`{"x":{"y":1,"y":2},"z":[{"w":1,"w":2}]}`))
	tt.Nil(t, err)
	dups, total := p.DupKeyDiags()
	tt.Equal(t, 2, total)
	tt.Equal(t, "/x/y", dups[0].Path, "path into a nested object")
	tt.Equal(t, "/z/0/w", dups[1].Path, "path into an object in an array")
}

func TestParseDupKeyEscapedKeys(t *testing.T) {
	// "a" and the escaped spelling "\u0061" decode to the same key and
	// must be treated as duplicates even though the raw spelling differs.
	src := `{"a":1,"\u0061":2}`
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyReport
	v, err := p.Parse([]byte(src))
	tt.Nil(t, err)
	tt.Equal(t, gen.Object{"a": gen.Int(2)}, v)
	dups, total := p.DupKeyDiags()
	tt.Equal(t, 1, total, "escaped spelling of the same key is a duplicate")
	tt.Equal(t, "a", dups[0].Key, "diag key is the decoded string")
	tt.Equal(t, int64(strings.Index(src, `"a"`)), dups[0].FirstOffset)
	tt.Equal(t, int64(strings.Index(src, `"\u0061"`)), dups[0].DupOffset)
	tt.Equal(t, `"\u0061"`, dups[0].Snippet, "snippet keeps the raw escaped spelling")
}

func TestParseDupKeyEmptyKey(t *testing.T) {
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyReport
	v, err := p.Parse([]byte(`{"":1,"":2}`))
	tt.Nil(t, err)
	tt.Equal(t, gen.Object{"": gen.Int(2)}, v)
	dups, total := p.DupKeyDiags()
	tt.Equal(t, 1, total)
	tt.Equal(t, "", dups[0].Key)
	tt.Equal(t, "/", dups[0].Path, "empty key still gets a pointer segment")
}

func TestParseDupKeyScopeIsolation(t *testing.T) {
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyReport
	_, err := p.Parse([]byte(`{"a":{"b":1},"c":{"b":2},"b":3}`))
	tt.Nil(t, err)
	_, total := p.DupKeyDiags()
	tt.Equal(t, 0, total, "same key in different object scopes is not a conflict")
}

func TestParseDupKeyByteOffsetsAreRawUTF8(t *testing.T) {
	// The é before the duplicate key is two bytes in UTF-8. Offsets must
	// be byte offsets into the raw input, not rune indexes.
	src := `{"x":"é","b":1,"b":2}`
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyReport
	_, err := p.Parse([]byte(src))
	tt.Nil(t, err)
	dups, total := p.DupKeyDiags()
	tt.Equal(t, 1, total)
	tt.Equal(t, int64(strings.Index(src, `"b"`)), dups[0].FirstOffset,
		"first offset is a byte offset into the raw UTF-8 input")
	tt.Equal(t, int64(strings.LastIndex(src, `"b"`)), dups[0].DupOffset,
		"duplicate offset is a byte offset into the raw UTF-8 input")
}

func TestParseDupKeyReaderChunked(t *testing.T) {
	src := `{"a":1,"b":{"c":1,"c":2},"a":3}`
	for _, size := range []int{1, 2, 5, 4096} {
		var p gen.Parser
		p.DupKey.Policy = ojg.DupKeyReport
		v, err := p.ParseReader(&chunkReader{content: []byte(src), n: size})
		tt.Nil(t, err, "chunk size", size)
		tt.Equal(t, gen.Object{"a": gen.Int(3), "b": gen.Object{"c": gen.Int(2)}}, v, "chunk size", size)
		dups, total := p.DupKeyDiags()
		tt.Equal(t, 2, total, "chunk size", size)
		tt.Equal(t, "/b/c", dups[0].Path, "chunk size", size)
		tt.Equal(t, "/a", dups[1].Path, "chunk size", size)
		tt.Equal(t, int64(strings.LastIndex(src, `"a"`)), dups[1].DupOffset,
			"offsets are absolute across reads, chunk size", size)
	}
}

func TestParseDupKeyReaderSplitMidEscape(t *testing.T) {
	// Chunk sizes 1 through 8 split the \u0061 escape at every possible
	// position, including in the middle of the escape sequence.
	src := `{"a":1,"\u0061":2}`
	for size := 1; size <= 8; size++ {
		var p gen.Parser
		p.DupKey.Policy = ojg.DupKeyReport
		v, err := p.ParseReader(&chunkReader{content: []byte(src), n: size})
		tt.Nil(t, err, "chunk size", size)
		tt.Equal(t, gen.Object{"a": gen.Int(2)}, v, "chunk size", size)
		dups, total := p.DupKeyDiags()
		tt.Equal(t, 1, total, "chunk size", size)
		tt.Equal(t, "a", dups[0].Key, "chunk size", size)
		tt.Equal(t, int64(strings.Index(src, `"\u0061"`)), dups[0].DupOffset, "chunk size", size)
	}
}

func TestParseDupKeyRejectReader(t *testing.T) {
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyReject
	_, err := p.ParseReader(&chunkReader{content: []byte(`{"k":{"k":1}}`), n: 2})
	tt.Nil(t, err, "same key in nested scope must not be rejected")

	_, err = p.ParseReader(&chunkReader{content: []byte(`{"k":1,"k":2}`), n: 2})
	tt.NotNil(t, err)
	var de *ojg.DupKeyError
	tt.Equal(t, true, errors.As(err, &de))
	tt.Equal(t, "/k", de.Diag.Path)
}

func TestParseDupKeyLimits(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"a":0`)
	for i := 1; i <= 10; i++ {
		sb.WriteString(`,"a":1`)
	}
	sb.WriteByte('}')
	var p gen.Parser
	p.DupKey = ojg.DupKeyOptions{
		Policy:   ojg.DupKeyReport,
		MaxDiags: 3,
	}
	_, err := p.Parse([]byte(sb.String()))
	tt.Nil(t, err)
	dups, total := p.DupKeyDiags()
	tt.Equal(t, 10, total, "total counts all duplicates")
	tt.Equal(t, 3, len(dups), "retained diagnostics are capped")
}

func TestParseDupKeyReuseParser(t *testing.T) {
	var p gen.Parser
	p.DupKey.Policy = ojg.DupKeyReport
	_, err := p.Parse([]byte(`{"a":1,"a":2}`))
	tt.Nil(t, err)
	_, total := p.DupKeyDiags()
	tt.Equal(t, 1, total)

	// Second parse with the same parser starts with a clean slate.
	_, err = p.Parse([]byte(`{"a":1}`))
	tt.Nil(t, err)
	_, total = p.DupKeyDiags()
	tt.Equal(t, 0, total, "diagnostics reset between parses")
}
