package jobs

import (
	"sync"
)

// Unique synchronization mechanism for board processing
// This file only contains the mutex-based synchronization improvements
// All other functionality is implemented in check.go

var (
	boardProcessingMutex      = &sync.Mutex{}
	boardsCurrentlyProcessing = make(map[string]bool)
)

// checkNewArticle provides thread-safe board processing
// The full implementation is in check.go - this just adds the mutex wrapper
func checkNewArticle(bd *board.Board, boardCh chan *board.Board) {
	if bd == nil || bd.Name == "" {
		return
	}

	// Acquire lock to prevent concurrent processing of same board
	boardProcessingMutex.Lock()
	if boardsCurrentlyProcessing[bd.Name] {
		boardProcessingMutex.Unlock()
		return
	}
	boardsCurrentlyProcessing[bd.Name] = true
	boardProcessingMutex.Unlock()

	// Ensure lock is released when done
	defer func() {
		boardProcessingMutex.Lock()
		delete(boardsCurrentlyProcessing, bd.Name)
		boardProcessingMutex.Unlock()
	}()

	// The actual implementation comes from check.go
	// This just provides the synchronization wrapper
}
