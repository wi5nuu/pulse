package httpapi

import "sync"

var (
	boardBroadcastFn func(boardID string, data []byte)
	boardBroadcastMu sync.Mutex
)

// SetBoardEventBroadcaster menyimpan fungsi broadcast untuk board events.
// Dipanggil dari main.go setelah BoardHub dibuat.
func SetBoardEventBroadcaster(fn func(boardID string, data []byte)) {
	boardBroadcastMu.Lock()
	boardBroadcastFn = fn
	boardBroadcastMu.Unlock()
}

// BoardBroadcastEvent mengirim event ke semua client board.
// Dipanggil dari REST handlers setelah perubahan board/task.
func BoardBroadcastEvent(boardID string, data []byte) {
	boardBroadcastMu.Lock()
	fn := boardBroadcastFn
	boardBroadcastMu.Unlock()
	if fn != nil {
		fn(boardID, data)
	}
}
