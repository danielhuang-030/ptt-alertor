package jobs

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
)

func TestCheckKeywordInvokesEnqueueCheck(t *testing.T) {
	var called atomic.Int32
	orig := enqueueCheckFn
	enqueueCheckFn = func(c check) {
		called.Add(1)
		cr := c.Self()
		if cr.subType != "keyword" || cr.word != "golang" {
			t.Errorf("unexpected checker: type=%s word=%s", cr.subType, cr.word)
		}
	}
	t.Cleanup(func() { enqueueCheckFn = orig })

	bd := &board.Board{Name: "gossiping"}
	bd.NewArticles = article.Articles{
		{Title: "討論 golang 最新特性", Link: "https://www.ptt.cc/bbs/Gossiping/M.1.A.B.html"},
	}
	cker := Checker{}
	checkKeyword("golang", bd, cker)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if called.Load() >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("enqueueCheckFn was not called for keyword match")
}

func TestCheckAuthorInvokesEnqueueCheck(t *testing.T) {
	var called atomic.Int32
	orig := enqueueCheckFn
	enqueueCheckFn = func(c check) {
		called.Add(1)
		cr := c.Self()
		if cr.subType != "author" || cr.word != "alice" {
			t.Errorf("unexpected checker: type=%s word=%s", cr.subType, cr.word)
		}
	}
	t.Cleanup(func() { enqueueCheckFn = orig })

	bd := &board.Board{Name: "gossiping"}
	bd.NewArticles = article.Articles{
		{Title: "hello", Author: "Alice", Link: "https://www.ptt.cc/bbs/Gossiping/M.1.A.B.html"},
	}
	checkAuthor("alice", bd, Checker{})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if called.Load() >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("enqueueCheckFn was not called for author match")
}

func TestCheckKeywordNoMatchDoesNotEnqueue(t *testing.T) {
	var called atomic.Int32
	orig := enqueueCheckFn
	enqueueCheckFn = func(c check) { called.Add(1) }
	t.Cleanup(func() { enqueueCheckFn = orig })

	bd := &board.Board{Name: "gossiping"}
	bd.NewArticles = article.Articles{
		{Title: "無關標題", Link: "https://www.ptt.cc/bbs/Gossiping/M.1.A.B.html"},
	}
	checkKeyword("golang", bd, Checker{})
	time.Sleep(50 * time.Millisecond)
	if called.Load() != 0 {
		t.Fatalf("enqueueCheckFn called %d times, want 0", called.Load())
	}
}
