package yws

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/pulse/server/internal/ycodec"
)

func newTestConn(role string) *Connection {
	return &Connection{
		doc:    &Document{},
		Role:   role,
		UserID: uuid.New().String(),
	}
}

func TestIsReadOnly(t *testing.T) {
	cases := []struct {
		role string
		want bool
	}{
		{role: "owner", want: false},
		{role: "editor", want: false},
		{role: "viewer", want: true}, // workspace viewer
		{role: "view", want: true},   // document share permission view
		{role: "", want: false},
		{role: "admin", want: false},
	}
	for _, c := range cases {
		if got := isReadOnly(c.role); got != c.want {
			t.Errorf("isReadOnly(%q) = %v, want %v", c.role, got, c.want)
		}
	}
}

func TestProcessor_ViewerCannotSendUpdate(t *testing.T) {
	// Fix IDOR: viewer share ("view") tidak boleh menulis.
	p := NewSyncProcessor(newTestStore(), nil)
	doc := newTestStore().GetOrCreate(uuid.New())

	viewer := newTestConn("view")
	viewer.doc = doc

	// Encode MsgSync(Update).
	updateBody := ycodec.WriteVarUint(nil, uint64(Update))
	updateBody = ycodec.WriteVarBuffer(updateBody, []byte("viewer-update"))
	msg := EncodeSyncMessage(updateBody)

	if err := p.Process(context.Background(), viewer, msg); err != nil {
		t.Fatalf("viewer update should be dropped silently, got error: %v", err)
	}
	if doc.PendingEventCount() != 0 {
		t.Fatal("viewer update must NOT be persisted as pending event")
	}
}

func TestProcessor_ViewerCannotSendSyncStep2(t *testing.T) {
	p := NewSyncProcessor(newTestStore(), nil)
	doc := newTestStore().GetOrCreate(uuid.New())

	viewer := newTestConn("viewer")
	viewer.doc = doc

	step2Body := ycodec.WriteVarUint(nil, uint64(SyncStep2))
	step2Body = ycodec.WriteVarBuffer(step2Body, []byte("viewer-state"))
	msg := EncodeSyncMessage(step2Body)

	if err := p.Process(context.Background(), viewer, msg); err != nil {
		t.Fatalf("viewer step2 should be ignored, got error: %v", err)
	}
	if _, ok := doc.State(); ok {
		t.Fatal("viewer must not be able to overwrite server state")
	}
}

func TestProcessor_EditorCanSendUpdate(t *testing.T) {
	p := NewSyncProcessor(newTestStore(), nil)
	doc := newTestStore().GetOrCreate(uuid.New())

	editor := newTestConn("editor")
	editor.doc = doc

	updateBody := ycodec.WriteVarUint(nil, uint64(Update))
	updateBody = ycodec.WriteVarBuffer(updateBody, []byte("editor-update"))
	msg := EncodeSyncMessage(updateBody)

	if err := p.Process(context.Background(), editor, msg); err != nil {
		t.Fatalf("editor update failed: %v", err)
	}
	if doc.PendingEventCount() != 1 {
		t.Fatalf("expected 1 pending event, got %d", doc.PendingEventCount())
	}
}

func TestEncodeRoleMessage(t *testing.T) {
	msg := EncodeRoleMessage("editor")

	msgType, n, err := ycodec.ReadVarUint(msg)
	if err != nil {
		t.Fatal("read message type:", err)
	}
	if byte(msgType) != MsgRole {
		t.Fatalf("message type = %d, want %d (MsgRole)", msgType, MsgRole)
	}
	if got := string(msg[n:]); got != "editor" {
		t.Fatalf("role payload = %q, want %q", got, "editor")
	}
}
