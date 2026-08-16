package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/pulse/server/internal/documents"
	"github.com/pulse/server/internal/models"
)

type SnapshotHandlers struct {
	snapshotRepo *documents.SnapshotRepo
	docRepo      *documents.Repo
}

func NewSnapshotHandlers(snapshotRepo *documents.SnapshotRepo, docRepo *documents.Repo) *SnapshotHandlers {
	return &SnapshotHandlers{
		snapshotRepo: snapshotRepo,
		docRepo:      docRepo,
	}
}

func (h *SnapshotHandlers) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid document id")
		return
	}
	// Authz: hanya member workspace dokumen yang boleh lihat riwayat.
	userID, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user in context")
		return
	}
	memberRole, err := h.docRepo.MemberRole(r.Context(), docID, userID)
	if err != nil || memberRole == "" {
		writeError(w, http.StatusForbidden, CodeForbidden, "not a workspace member")
		return
	}
	snapshots, err := h.snapshotRepo.ListByDocument(r.Context(), docID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not list snapshots")
		return
	}
	if snapshots == nil {
		snapshots = []documents.SnapshotInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": snapshots})
}

func (h *SnapshotHandlers) RestoreSnapshot(w http.ResponseWriter, r *http.Request) {
	docID, err := uuid.Parse(chi.URLParam(r, "documentID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid document id")
		return
	}
	snapshotIDStr := chi.URLParam(r, "snapshotID")
	snapshotID, err := strconv.ParseInt(snapshotIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeBadRequest, "invalid snapshot id")
		return
	}

	userID, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, CodeUnauthorized, "no user in context")
		return
	}

	memberRole, err := h.docRepo.MemberRole(r.Context(), docID, userID)
	if err != nil || memberRole == "" {
		writeError(w, http.StatusForbidden, CodeForbidden, "not a workspace member")
		return
	}
	if memberRole != models.RoleOwner && memberRole != models.RoleEditor {
		writeError(w, http.StatusForbidden, CodeForbidden, "only owner/editor can restore")
		return
	}

	snapshot, err := h.snapshotRepo.GetByID(r.Context(), snapshotID)
	if err != nil {
		if errors.Is(err, documents.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeNotFound, "snapshot not found")
			return
		}
		writeError(w, http.StatusInternalServerError, CodeInternal, "could not get snapshot")
		return
	}

	// FIX IDOR (kritis): verifikasi snapshot milik dokumen yang diminta.
	// Sebelumnya snapshot di-fetch hanya by ID tanpa cek kepemilikan —
	// owner/editor dokumen A yang mengetahui ID snapshot dokumen B bisa
	// menimpa state dokumen A dengan snapshot B.
	if snapshot.DocumentID != docID {
		writeError(w, http.StatusNotFound, CodeNotFound, "snapshot not found for document")
		return
	}

	restoreUserID := &userID
	if err := h.snapshotRepo.SaveSnapshot(r.Context(), docID, snapshot.State, snapshot.EventCount, restoreUserID); err != nil {
		slog.Error("failed to save restore snapshot marker", "error", err)
	}

	DocStateBroadcast(docID, snapshot.State)

	writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}
