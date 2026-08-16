package httpapi

import (
	"sync"

	"github.com/google/uuid"
)

var (
	docStateBroadcastFn func(docID uuid.UUID, data []byte)
	docStateBroadcastMu sync.RWMutex

	docEventBroadcastFn func(docID uuid.UUID, data []byte)
	docEventBroadcastMu sync.RWMutex
)

func SetDocStateBroadcaster(fn func(docID uuid.UUID, data []byte)) {
	docStateBroadcastMu.Lock()
	docStateBroadcastFn = fn
	docStateBroadcastMu.Unlock()
}

func DocStateBroadcast(docID uuid.UUID, data []byte) {
	docStateBroadcastMu.RLock()
	fn := docStateBroadcastFn
	docStateBroadcastMu.RUnlock()
	if fn != nil {
		fn(docID, data)
	}
}

// SetDocEventBroadcaster wiring dari main: broadcast event dokumen (komentar
// dll) ke semua koneksi WS dokumen. Client menginterpretasi payload JSON.
func SetDocEventBroadcaster(fn func(docID uuid.UUID, data []byte)) {
	docEventBroadcastMu.Lock()
	docEventBroadcastFn = fn
	docEventBroadcastMu.Unlock()
}

func DocEventBroadcast(docID uuid.UUID, data []byte) {
	docEventBroadcastMu.RLock()
	fn := docEventBroadcastFn
	docEventBroadcastMu.RUnlock()
	if fn != nil {
		fn(docID, data)
	}
}
