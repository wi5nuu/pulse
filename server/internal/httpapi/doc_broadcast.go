package httpapi

import (
	"github.com/google/uuid"
)

var (
	docStateBroadcastFn func(docID uuid.UUID, data []byte)
)

func SetDocStateBroadcaster(fn func(docID uuid.UUID, data []byte)) {
	docStateBroadcastFn = fn
}

func DocStateBroadcast(docID uuid.UUID, data []byte) {
	if docStateBroadcastFn != nil {
		docStateBroadcastFn(docID, data)
	}
}
