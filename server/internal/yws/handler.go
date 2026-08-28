// Handler HTTP untuk upgrade WebSocket di /ws/doc/{id}.
//
// Acceptance: client connect, handshake sync, edit di satu tab muncul di tab lain.
package yws

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/pulse/server/internal/comments"
	"github.com/pulse/server/internal/documents"
	"github.com/pulse/server/internal/httpapi"
)

// WSHandler menangani upgrade koneksi WebSocket untuk dokumen.
type WSHandler struct {
	store    *Store
	docs     *documents.Repo
	comRepo  *comments.Repo
	snapRepo *documents.SnapshotRepo
	logger   *slog.Logger
	upgrader websocket.Upgrader
}

func NewWSHandler(store *Store, docs *documents.Repo, snapRepo *documents.SnapshotRepo, allowedOrigin string, logger *slog.Logger) *WSHandler {
	return NewWSHandlerWithComments(store, docs, nil, snapRepo, allowedOrigin, logger)
}

func NewWSHandlerWithComments(store *Store, docs *documents.Repo, comRepo *comments.Repo, snapRepo *documents.SnapshotRepo, allowedOrigin string, logger *slog.Logger) *WSHandler {
	if logger == nil {
		logger = slog.Default()
	}
	// Upgrader: izinkan origin frontend. CheckOrigin eksplisit supaya tidak
	// default-accept cross-origin di produksi.
	allowed := map[string]bool{allowedOrigin: true}
	return &WSHandler{
		store:    store,
		docs:     docs,
		snapRepo: snapRepo,
		logger:   logger,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// subprotocols tidak di-require; pakai default.
			CheckOrigin: func(r *http.Request) bool {
				return allowed[r.Header.Get("Origin")]
			},
			// HandshakeTimeout: batasi waktu handshake agar koneksi idle tidak numpuk.
			HandshakeTimeout: 10 * time.Second,
		},
	}
}

// ServeHTTP: chi-compatible signature.
//
// Path: /ws/doc/{document_id}
// Auth: Bearer token via ?token= query (WebSocket API browser TIDAK support
//       header Authorization kustom — jadi pakai query param. Token ini adalah
//       JWT access token pendek. Trade-off security: token muncul di URL →
//       bisa ter-log di access log. Mitigasi: gunakan token pendek (15 menit),
//       dan jangan log query string di access logger produksi.)
func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Parse & validasi document_id dari path.
	docIDStr := chi.URLParam(r, "documentID")
	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		httpapi.NewWSWriteError(w, http.StatusBadRequest, "invalid document id")
		return
	}

	// 2. Auth: ambil token dari query param. (Alternatif: cookie, tapi WebSocket
	//    di browser otomatis kirim cookie same-origin — lebih aman. Kami pilih
	//    query param karena access token disimpan in-memory JS, bukan cookie.)
	token := r.URL.Query().Get("token")
	if token == "" {
		httpapi.NewWSWriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	claims, err := httpapi.VerifyWSAccessToken(r.Context(), token)
	if err != nil {
		httpapi.NewWSWriteError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	userID := claims.UserID

	// 3. Authorization: check if user has access (workspace member OR direct
	//    share OR link share token "ls"). Link share = "Anyone with the link"
	//    (fiturwajibada H.168) — permission view/edit dari token.
	hasAccess, permission, err := h.docs.HasDocumentAccess(r.Context(), docID, userID)
	if err != nil {
		h.logger.Error("ws access check failed", "error", err, "doc", docID, "user", userID)
		httpapi.NewWSWriteError(w, http.StatusInternalServerError, "access check failed")
		return
	}
	lsToken := r.URL.Query().Get("ls")
	usedLinkShare := false
	if !hasAccess {
		if lsToken != "" && h.comRepo != nil {
			ls, err := h.comRepo.GetByToken(r.Context(), lsToken)
			if err == nil && ls.DocumentID == docID {
				hasAccess = true
				permission = ls.Permission
				usedLinkShare = true
			}
		}
	}
	if !hasAccess {
		httpapi.NewWSWriteError(w, http.StatusForbidden, "no access to document")
		return
	}

	// Use permission as role (owner/editor/viewer or view/edit from share).
	// For link shares, use the permission directly (view/edit).
	// For document shares, use the permission (view/edit).
	// For workspace members, use the role (owner/editor/viewer).
	role := permission
	if !usedLinkShare && permission != "view" && permission != "edit" {
		// This is a workspace role (owner/editor/viewer), keep as-is
		role = permission
	}
	// Validate link share permission: "view" should be read-only
	if usedLinkShare && permission == "view" {
		role = "view"
	}

	// 4. Upgrade ke WebSocket.
	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade sudah menulis response kalau gagal; tidak perlu tulis lagi.
		h.logger.Debug("ws upgrade failed", "error", err)
		return
	}

	// 5. Setup koneksi & register ke document store.
	doc := h.store.GetOrCreate(docID)

	// 5a. Fase 4 — load state dari DB jika dokumen belum punya state in-memory.
	//     Cari snapshot terbaru + events setelahnya, set sebagai lastState + replayEvents.
	if _, hasState := doc.State(); !hasState && h.snapRepo != nil {
		if snap, err := h.snapRepo.GetLatestSnapshot(r.Context(), docID); err == nil {
			doc.SetState(snap.State)
			if events, err := h.snapRepo.LoadEventsSince(r.Context(), docID, snap.CreatedAt); err == nil && len(events) > 0 {
				doc.SetReplayEvents(events)
				h.logger.Info("loaded replay events from DB",
					"doc", docID, "count", len(events))
			}
		}
	}

	conn := NewConnection(ConnConfig{
		WS:        ws,
		Doc:       doc,
		UserID:    userID.String(),
		UserName:  claims.Name,
		Role:      role,
	})
	doc.AddConnection(conn)

	// FIX viewer read-only (UI): kirim role ke client saat connect supaya
	// client bisa render editor sebagai read-only untuk viewer (share "view"
	// atau workspace viewer). Pesan: MsgRole (byte 5) + role string.
	roleMsg := EncodeRoleMessage(conn.Role)
	conn.Send(roleMsg)

	h.logger.Info("ws connected",
		"doc", docID, "user", userID, "role", role,
		"connections", doc.ConnectionCount())

	// 6. Jalankan read & write pump. ctx diturunkan dari request; tapi request
	//    ctx mati saat handler return — sedangkan kami ingin koneksi hidup setelah
	//    handler return. Jadi pakai background context dengan cancel manual.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proc := NewSyncProcessor(h.store, h.logger.With("component", "syncproc"))

	// Cleanup saat kedua pump selesai.
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		conn.WritePump(ctx)
	}()

	// Read pump blocking. Saat return → tutup & hapus dari doc.
	readErr := conn.ReadPump(ctx, proc)
	doc.RemoveConnection(conn)
	conn.Close()
	<-pumpDone
	// Bersihkan dokumen yang tidak terpakai lagi (tanpa pending data).
	h.store.MaybeEvict(docID)

	h.logger.Info("ws disconnected",
		"doc", docID, "user", userID,
		"connections", doc.ConnectionCount(),
		"readErr", readErr)
}
