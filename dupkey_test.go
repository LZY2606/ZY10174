// Copyright (c) 2026, Peter Ohler, All rights reserved.

package ojg_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ohler55/ojg"
	"github.com/ohler55/ojg/tt"
)

func TestDupKeyPolicyDefaults(t *testing.T) {
	var tracker ojg.DupTracker
	tracker.Reset(ojg.DupKeyOptions{})
	tt.Equal(t, false, tracker.Enabled(), "zero policy should keep default behavior")
	tt.Equal(t, false, tracker.TrackDiag(), "zero policy should not track diagnostics")

	tracker.Reset(ojg.DupKeyOptions{Policy: ojg.DupKeyPolicy(99)})
	tt.Equal(t, false, tracker.Enabled(), "invalid policy should fall back to keep-last")

	tracker.Reset(ojg.DupKeyOptions{Policy: ojg.DupKeyPolicy(-1)})
	tt.Equal(t, false, tracker.Enabled(), "negative policy should fall back to keep-last")
}

func TestDupTrackerFirstSkipAssign(t *testing.T) {
	var tracker ojg.DupTracker
	tracker.Reset(ojg.DupKeyOptions{Policy: ojg.DupKeyFirst})
	tracker.Open(false)
	tt.Nil(t, tracker.KeyCompleted("a", 1, `"a"`))
	tt.Equal(t, false, tracker.SkipAssign(), "first occurrence must be assigned")
	tt.Nil(t, tracker.KeyCompleted("a", 9, `"a"`))
	tt.Equal(t, true, tracker.SkipAssign(), "duplicate must be skipped with keep-first")
	tt.Equal(t, false, tracker.SkipAssign(), "skip flag must be consumed once")
}

func TestDupTrackerScopes(t *testing.T) {
	var tracker ojg.DupTracker
	tracker.Reset(ojg.DupKeyOptions{Policy: ojg.DupKeyReport})
	tracker.Open(false)
	tt.Nil(t, tracker.KeyCompleted("a", 1, `"a"`))
	tracker.Open(false) // nested object value of "a"
	tt.Nil(t, tracker.KeyCompleted("a", 7, `"a"`), "same key in a nested object is not a duplicate")
	tracker.Close()
	tt.Nil(t, tracker.KeyCompleted("a", 12, `"a"`))
	tracker.Close()
	dups, total := tracker.Diags()
	tt.Equal(t, 1, total, "only the repeated key in the same object counts")
	tt.Equal(t, 1, len(dups))
	tt.Equal(t, int64(1), dups[0].FirstOffset)
	tt.Equal(t, int64(12), dups[0].DupOffset)
}

func TestDupTrackerPath(t *testing.T) {
	var tracker ojg.DupTracker
	tracker.Reset(ojg.DupKeyOptions{Policy: ojg.DupKeyReport})
	tracker.Open(false) // root object
	tracker.KeyCompleted("x", 1, `"x"`)
	tracker.Open(true)   // array value of "x"
	tracker.ValueAdded() // first element
	tracker.Open(false)  // second element is an object
	tracker.KeyCompleted("y/k", 20, `"y/k"`)
	tracker.KeyCompleted("y/k", 30, `"y/k"`)
	dups, total := tracker.Diags()
	tt.Equal(t, 1, total)
	tt.Equal(t, "/x/1/y~1k", dups[0].Path, "path must be a JSON Pointer with escaped segments")
	tt.Equal(t, "y/k", dups[0].Key, "key must be the decoded key")
}

func TestDupTrackerPathTildeEscape(t *testing.T) {
	var tracker ojg.DupTracker
	tracker.Reset(ojg.DupKeyOptions{Policy: ojg.DupKeyReport})
	tracker.Open(false)
	tracker.KeyCompleted("~", 1, `"~"`)
	tracker.KeyCompleted("~", 5, `"~"`)
	dups, _ := tracker.Diags()
	tt.Equal(t, "/~0", dups[0].Path, "tilde must be escaped as ~0")
}

func TestDupTrackerReject(t *testing.T) {
	var tracker ojg.DupTracker
	tracker.Reset(ojg.DupKeyOptions{Policy: ojg.DupKeyReject})
	tracker.Open(false)
	tt.Nil(t, tracker.KeyCompleted("a", 1, `"a"`))
	err := tracker.KeyCompleted("a", 9, `"a"`)
	tt.NotNil(t, err)
	de, ok := err.(*ojg.DupKeyError)
	tt.Equal(t, true, ok, "reject error must be a *ojg.DupKeyError")
	tt.Equal(t, "/a", de.Diag.Path)
	tt.Equal(t, "a", de.Diag.Key)
	tt.Equal(t, int64(1), de.Diag.FirstOffset)
	tt.Equal(t, int64(9), de.Diag.DupOffset)
	tt.Equal(t, true, strings.Contains(err.Error(), `"a"`), "error message should include the key")
}

func TestDupTrackerLimits(t *testing.T) {
	var tracker ojg.DupTracker
	var called int
	tracker.Reset(ojg.DupKeyOptions{
		Policy:   ojg.DupKeyReport,
		MaxDiags: 2,
		OnDup:    func(ojg.DupKeyDiag) { called++ },
	})
	tracker.Open(false)
	tracker.KeyCompleted("a", 1, `"a"`)
	for i := 0; i < 5; i++ {
		tracker.KeyCompleted("a", int64(1+i*4), `"a"`)
	}
	dups, total := tracker.Diags()
	tt.Equal(t, 5, total, "total counts all duplicates")
	tt.Equal(t, 2, len(dups), "retained diagnostics are capped by MaxDiags")
	tt.Equal(t, 5, called, "OnDup sees every duplicate")
	tt.Equal(t, int64(1), dups[1].FirstOffset, "first offset stays the first occurrence")
}

func TestDupTrackerPathAndSnippetTruncation(t *testing.T) {
	var tracker ojg.DupTracker
	tracker.Reset(ojg.DupKeyOptions{
		Policy:     ojg.DupKeyReport,
		MaxPath:    10,
		MaxSnippet: 5,
	})
	tracker.Open(false)
	long := strings.Repeat("k", 100)
	tracker.KeyCompleted(long, 1, `"`+long+`"`)
	tracker.KeyCompleted(long, 200, `"`+long+`"`)
	dups, _ := tracker.Diags()
	tt.Equal(t, 1, len(dups))
	if 10 < len(dups[0].Path) {
		t.Fatalf("path length %d exceeds MaxPath 10: %q", len(dups[0].Path), dups[0].Path)
	}
	tt.Equal(t, true, strings.HasSuffix(dups[0].Path, "..."), "truncated path should end with an ellipsis")
	if 5 < len(dups[0].Snippet) {
		t.Fatalf("snippet length %d exceeds MaxSnippet 5: %q", len(dups[0].Snippet), dups[0].Snippet)
	}
	tt.Equal(t, long, dups[0].Key, "key is never truncated")
}

func TestDupTrackerTruncationRuneSafe(t *testing.T) {
	var tracker ojg.DupTracker
	tracker.Reset(ojg.DupKeyOptions{
		Policy:  ojg.DupKeyReport,
		MaxPath: 6, // cuts into the multibyte é
	})
	tracker.Open(false)
	tracker.KeyCompleted("éé", 1, `"éé"`)
	tracker.KeyCompleted("éé", 10, `"éé"`)
	dups, _ := tracker.Diags()
	tt.Equal(t, 1, len(dups))
	tt.Equal(t, true, utf8.ValidString(dups[0].Path), "truncated path must stay valid UTF-8")
}

func TestDupTrackerUnlimited(t *testing.T) {
	var tracker ojg.DupTracker
	tracker.Reset(ojg.DupKeyOptions{
		Policy:   ojg.DupKeyReport,
		MaxDiags: -1,
	})
	tracker.Open(false)
	tracker.KeyCompleted("a", 1, `"a"`)
	for i := 0; i < 200; i++ {
		tracker.KeyCompleted("a", int64(1+i*4), `"a"`)
	}
	dups, total := tracker.Diags()
	tt.Equal(t, 200, total)
	tt.Equal(t, 200, len(dups), "negative MaxDiags removes the limit")
}

func TestDupTrackerResetReuse(t *testing.T) {
	var tracker ojg.DupTracker
	tracker.Reset(ojg.DupKeyOptions{Policy: ojg.DupKeyReport})
	tracker.Open(false)
	tracker.KeyCompleted("a", 1, `"a"`)
	// Simulate an aborted parse: containers left open.
	tracker.Reset(ojg.DupKeyOptions{Policy: ojg.DupKeyReport})
	tracker.Open(false)
	tracker.KeyCompleted("b", 1, `"b"`)
	tracker.KeyCompleted("b", 5, `"b"`)
	tracker.Close()
	dups, total := tracker.Diags()
	tt.Equal(t, 1, total, "tracker must be reusable after reset")
	tt.Equal(t, "/b", dups[0].Path)
}
