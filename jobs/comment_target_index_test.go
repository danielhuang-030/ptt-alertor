package jobs

import (
	"testing"
)

func TestCommentTargetIndexHasBoardFromMemory(t *testing.T) {
	idx := NewCommentTargetIndex(func() map[string][]string {
		return map[string][]string{
			"gossiping": {"M.1.A.B"},
			"lol":       {},
		}
	})
	idx.Refresh()
	if !idx.HasBoard("Gossiping") {
		t.Fatal("HasBoard gossiping")
	}
	if idx.HasBoard("lol") {
		t.Fatal("empty codes should be false")
	}
	codes := idx.CodesForBoard("gossiping")
	if len(codes) != 1 || codes[0] != "M.1.A.B" {
		t.Fatalf("codes=%v", codes)
	}
}

func TestDefaultBoardHasCommentTargetsUsesIndex(t *testing.T) {
	orig := commentTargetIndex
	t.Cleanup(func() { commentTargetIndex = orig })

	idx := NewCommentTargetIndex(func() map[string][]string {
		return map[string][]string{"gossiping": {"X"}}
	})
	idx.Refresh()
	SetCommentTargetIndex(idx)

	if !defaultBoardHasCommentTargets("gossiping") {
		t.Fatal("expected true from cache")
	}
	if defaultBoardHasCommentTargets("other") {
		t.Fatal("expected false")
	}
}
