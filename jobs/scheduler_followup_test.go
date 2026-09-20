package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
)

func TestFollowUpWorkDoesNotPanic(t *testing.T) {
	origPush, origCmt := runPushSumBoardFn, runCommentBoardFn
	runPushSumBoardFn = func(string) {}
	runCommentBoardFn = func(string) {}
	t.Cleanup(func() {
		runPushSumBoardFn, runCommentBoardFn = origPush, origCmt
	})

	s := NewScheduler(SchedulerConfig{Workers: 1, QueueSize: 8, ActiveTick: time.Second, IdlePoll: time.Second},
		func() bool { return true },
		func() []string { return nil },
	)
	s.SetRefresher(&countingRefresher{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	if s.queue.TryEnqueue(WorkItem{Kind: WorkCheckPushSum, Board: "gossiping"}) != EnqueueOK {
		t.Fatal("enqueue pushsum")
	}
	if s.queue.TryEnqueue(WorkItem{Kind: WorkCheckComment, Board: "gossiping"}) != EnqueueOK {
		t.Fatal("enqueue comment on same board should work with kind+board key")
	}
	time.Sleep(200 * time.Millisecond)
	s.Stop()
}

func TestRefreshEnqueuesFollowUpsWhenTargetsExist(t *testing.T) {
	var pushSumRuns, commentRuns atomic.Int32
	origPush, origCmt := boardHasPushSumFn, boardHasCommentTargetsFn
	origRunPush, origRunCmt := runPushSumBoardFn, runCommentBoardFn
	boardHasPushSumFn = func(board string) bool { return board == "gossiping" }
	boardHasCommentTargetsFn = func(board string) bool { return board == "gossiping" }
	runPushSumBoardFn = func(board string) { pushSumRuns.Add(1) }
	runCommentBoardFn = func(board string) { commentRuns.Add(1) }
	t.Cleanup(func() {
		boardHasPushSumFn, boardHasCommentTargetsFn = origPush, origCmt
		runPushSumBoardFn, runCommentBoardFn = origRunPush, origRunCmt
	})

	s := NewScheduler(SchedulerConfig{Workers: 1, QueueSize: 16, ActiveTick: time.Second, IdlePoll: time.Second},
		func() bool { return true },
		func() []string { return []string{"gossiping"} },
	)
	s.SetRefresher(&countingRefresher{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	s.TickOnce(time.Now())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pushSumRuns.Load() >= 1 && commentRuns.Load() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.Stop()
	if pushSumRuns.Load() < 1 {
		t.Fatal("pushsum follow-up not run")
	}
	if commentRuns.Load() < 1 {
		t.Fatal("comment follow-up not run")
	}
}

func TestRefreshSkipsFollowUpsWithoutTargets(t *testing.T) {
	var pushSumRuns atomic.Int32
	origPush, origCmt := boardHasPushSumFn, boardHasCommentTargetsFn
	origRunPush := runPushSumBoardFn
	boardHasPushSumFn = func(string) bool { return false }
	boardHasCommentTargetsFn = func(string) bool { return false }
	runPushSumBoardFn = func(string) { pushSumRuns.Add(1) }
	t.Cleanup(func() {
		boardHasPushSumFn, boardHasCommentTargetsFn = origPush, origCmt
		runPushSumBoardFn = origRunPush
	})

	s := NewScheduler(SchedulerConfig{Workers: 1, QueueSize: 8, ActiveTick: time.Second, IdlePoll: time.Second},
		func() bool { return true },
		func() []string { return []string{"lol"} },
	)
	s.SetRefresher(&countingRefresher{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	s.TickOnce(time.Now())
	time.Sleep(300 * time.Millisecond)
	s.Stop()
	if pushSumRuns.Load() != 0 {
		t.Fatalf("pushsum runs=%d want 0", pushSumRuns.Load())
	}
}

// I10: stub refresh that produces a keyword match must invoke enqueueCheckFn (not merely avoid panic).
func TestNotifyContract_StubRefreshMatchEnqueuesCheck(t *testing.T) {
	var called atomic.Int32
	orig := enqueueCheckFn
	enqueueCheckFn = func(c check) { called.Add(1) }
	t.Cleanup(func() { enqueueCheckFn = orig })

	s := NewScheduler(SchedulerConfig{Workers: 1, QueueSize: 8, ActiveTick: time.Second, IdlePoll: time.Second},
		func() bool { return true },
		func() []string { return []string{"gossiping"} },
	)
	s.SetRefresher(BoardRefresherFunc(func(boardName string) error {
		bd := &board.Board{Name: boardName}
		bd.NewArticles = article.Articles{
			{Title: "討論 golang 最新特性", Link: "https://www.ptt.cc/bbs/Gossiping/M.1.A.B.html"},
		}
		checkKeyword("golang", bd, Checker{})
		return nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	s.TickOnce(time.Now())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if called.Load() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.Stop()
	if called.Load() < 1 {
		t.Fatal("enqueueCheckFn not called after stub refresh match")
	}
}
