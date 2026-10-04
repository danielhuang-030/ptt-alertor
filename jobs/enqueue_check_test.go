package jobs

import (
	"testing"
	"time"
)

func TestEnqueueCheckDoesNotPanic(t *testing.T) {
	done := make(chan struct{})
	go func() {
		enqueueCheck(Checker{})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("enqueueCheck blocked")
	}
}
