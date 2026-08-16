# PULSE — MASTER BUILD PROMPT v4.0 (Enterprise, 20-Tahun Horizon)

> Dokumen ini adalah satu-satunya sumber kebenaran (*single source of truth*) untuk Pulse —
> platform kolaborasi real-time kelas enterprise (dokumen, board, presence, sharing).
> Dibuat untuk bertahan & berevolusi 20 tahun ke depan: dirancang agar setiap keputusan
> arsitektur dapat dilacak balik ke persyaratan (requirement traceability), dapat diuji,
> dapat di-deploy ke multi-environment, dan dapat diskalakan dari 10 user → 10 juta user.
>
> **Cara pakai:** salin seluruh isi ke agent builder (Claude Code / opencode / dll). Agent
> TIDAK boleh mengubah dokumen ini sendiri. Semua perubahan lewat diskusi eksplisit.
> Bagian yang sudah ditandai **[DONE]** berarti sudah diimplementasi & terverifikasi di repo —
> agent membacanya sebagai baseline, bukan pekerjaan baru.

---

## 0. Ringkasan Eksekutif

Pulse adalah **real-time collaborative workspace**: dokumen rich-text kolaboratif
(multi-cursor, selective undo), task board drag-and-drop multi-user, live presence,
version history, sharing per-dokumen (seperti Google Docs), dan auth JWT yang aman.

### 0.1 Visi 20 Tahun (Horizon 2036+)

| Horizon | Tema | Target Kapabilitas |
|---|---|---|
| H1 (2026–2028) | Foundation | 1k CCU per server, HA 99.9%, zero data loss, compliance dasar |
| H2 (2029–2032) | Scale | 100k CCU multi-region, Redis cluster + Pub/Sub sharding, observability penuh, RBAC/SSO |
| H3 (2033–2036) | Evolve | Serverless/CSS (CRDT-as-a-Service), AI copilot, e2ee opsional, air-gap deploy |

### 0.2 Prinsip Arsitektur (Non-Negotiable)

1. **CRDT-first**: semua state kolaboratif memakai Yjs di client; server = relay cerdas
   + persistence. BUKAN re-implementasi CRDT di server (§4.3).
2. **No data loss**: setiap update yang diterima server WAJIB akhirnya sampai ke Postgres.
   Dalam perjalanan boleh di-buffer, TIDAK boleh di-drop.
3. **Server-enforced authorization**: semua aturan akses divalidasi ulang di server
   (UI read-only hanyalah UX, bukan keamanan).
4. **Fail fast di startup**: konfigurasi salah / DB tidak reachable → mati cepat, bukan
   berjalan rusak.
5. **Idempotent everywhere**: WS reconnect, event replay, snapshot restore — semua
   operasi harus aman dijalankan ulang tanpa duplikasi/konflik (CRDT menjamin ini).
6. **Observable by default**: log terstruktur + request ID di setiap request, health
   endpoint yang jujur, metrik dasar di metric endpoint.
7. **Evolvability**: setiap abstraksi (repo, provider, store) punya satu tanggung jawab;
   tidak ada god object; komentar kode menjelaskan rationale, bukan "apa".

---

## 1. Tech Stack (Final — Tidak Boleh Diganti Tanpa Diskusi)

| Layer | Teknologi | Catatan Evolusi (20 tahun) |
|---|---|---|
| Backend REST & WS | Go 1.25+ (`chi/v5`, `gorilla/websocket`) | Go menjaga binary small + GC predictable; siap migrasi ke stdlib `net/http` routing saat butuh |
| CRDT | Yjs v13 + `y-protocols` (client); `y-prosemirror` | Yjs adalah standar de-facto; abstraksi provider kami (`PulseWSProvider`) memungkinkan ganti transport tanpa sentuh editor |
| Database | PostgreSQL 18+ (lokal), pgx v5 | Partisi, logical replication, dan `pg_terminate_backend` sudah teruji; siap upgrade minor tanpa breaking |
| Cache / Pub-Sub | Redis 7+ (URL tersedia di config) | Saat multi-instance tiba: Redis Pub/Sub untuk broadcast lintas node |
| Frontend | Next.js 14+ App Router, TypeScript strict | App Router stabil; rute bisa di-upgrade bertahap ke versi Next terbaru |
| Editor | ProseMirror + `y-prosemirror` + `prosemirror-example-setup` | Plugin editor terisolasi di satu file — mudah diganti/ditambah plugin |
| State (non-CRDT) | Zustand (auth store) | Kecil, tidak mengikat arsitektur |
| Styling | Tailwind CSS | Utility-first — konsisten dengan sistem desain §5 |
| Auth | JWT access + refresh token (httpOnly cookie) | Tidak menyimpan JWT di localStorage (XSS-safe) |
| Container | Docker + docker-compose | Prod multi-stage build; dev hot-reload |
| Test | Go `testing` (stdlib); Playwright untuk E2E (target) | Tidak ada framework berat; CI sederhana |
| Observability | structured `slog` JSON + health/ready endpoint + request ID | Jalur siap dipasangi Prometheus/OTel tanpa refactor besar |

### 1.1 Konfigurasi (12-Factor)

Semua konfigurasi lewat environment variable; `.env` hanya untuk dev (load by `godotenv`).

| Var | Wajib | Default | Deskripsi |
|---|---|---|---|
| `ENV` | — | `dev` | `dev` \| `prod`; memicu validasi secret panjang |
| `PORT` | — | `8080` | Port HTTP+WS |
| `DATABASE_URL` | ✅ | — | DSN Postgres (password di-mask saat di-log) |
| `REDIS_URL` | ✅ | — | DSN Redis |
| `JWT_SECRET` | ✅ | — | ≥32 char di produksi (ditolak saat startup jika pendek) |
| `JWT_ACCESS_TTL` | — | `15m` | Umur access token |
| `REFRESH_TTL` | — | `720h` | Umur refresh token |
| `CORS_ORIGIN` | — | `http://localhost:3000` | Origin yang diizinkan |

**Pool DB** (sudah dikonfigurasi di `db.New`): MaxConns 25, MinConns 2, MaxConnLifetime 1h,
MaxConnIdleTime 15m, statement_timeout 10s, lock_timeout 5s — mencegah slow query
menjegal pool.

---

## 2. Daftar Fitur Lengkap (Matriks Maturity)

Skala status: **[DONE]** = implementasi terverifikasi, **[CORE]** = wajib di fase ini,
**[S1]** = horizon 1, **[S2]** = horizon 2, **[S3]** = horizon 3.

### 2.1 Auth & User **[DONE]**
- [x] Register (email, password, display name) — validasi strength minimal
- [x] Login / logout
- [x] Refresh token flow otomatis (silent refresh saat access expired, revocable)
- [x] Password hashing (bcrypt via `auth/password.go`)
- [x] Update profil (nama) — `PATCH /api/users/me` + alias `/users/me` (backward-compat)
- [ ] Reset password via email **[S1]**
- [ ] 2FA (TOTP) **[S1]**
- [ ] SSO / OIDC (Google, Entra ID) **[S2]**

### 2.2 Workspace & Membership **[DONE]**
- [x] Create workspace, list workspace user
- [x] Invite via email + invite link token (expirable, accept/reject)
- [x] List member & role (owner/editor/viewer)
- [x] Ubah role member (hanya owner)
- [x] Remove member
- [x] Switch antar workspace
- [x] Pending invitations page + notification bell
- [ ] Workspace settings: rename, transfer ownership, arsip **[S1]**
- [ ] Audit log aktivitas member (siapa melakukan apa, kapan) **[S1]**
- [ ] SAML/SCIM provisioning **[S2]**

### 2.3 Document Collaboration (Tier 1 — Core) **[DONE]**
- [x] Create/rename/delete dokumen dalam workspace (rename inline di sidebar)
- [x] Rich-text: heading, paragraph, bullet/numbered list, bold/italic/underline, code block
- [x] Real-time multi-user editing (relay WS + Yjs sync dua arah)
- [x] Live cursor per user (warna unik + label nama)
- [x] Avatar presence list
- [x] Selective undo/redo per-user origin (Y.UndoManager + trackedOrigins)
- [x] Auto-save via event log (tidak ada tombol save manual)
- [x] Version history: snapshot timeline, restore
- [x] Read-only mode untuk role viewer & share-"view" (**enforced di server**)
- [x] Indikator status koneksi (connected/reconnecting/offline) + heartbeat
- [x] Reconnection otomatis (exponential backoff) + resync penuh + **resync dua arah**
  (edit yang dibuat offline TIDAK hilang — server minta state client balik, fix C2)
- [x] **Document sharing per-dokumen** (email lookup, permission view/edit, list shares,
  unshare, "Shared with me" di sidebar, "shared documents" di workspace) **[DONE]**
- [ ] Komentar & mention (@user) di dokumen **[S1]**
- [ ] Mode tontonan (view-only broadcast mode untuk demo/presentasi) **[S1]**
- [ ] Slash command + template dokumen **[S1]**
- [ ] Offline-first editing (PWA + IndexedDB) **[S2]**
- [ ] AI copilot (ringkasan, tulis lanjut) **[S3]**

### 2.4 Task Board (Tier 2) **[DONE]**
- [x] Create board dalam workspace
- [x] Create/rename/delete column
- [x] Create/edit/delete task (title; optimistik update + version check)
- [x] Drag-and-drop task antar kolom + reorder (fractional indexing, float position)
- [x] Real-time reflect via WS `BOARD_EVENT` (broadcast JSON)
- [x] Optimistic concurrency: `UPDATE ... WHERE version = ?` → retry/reconcile
- [ ] Assignee UI + filter (hanya kolom assignee_id di DB) **[S1]**
- [ ] Deskripsi task (rich text) + checklist **[S1]**
- [ ] Label / due date / priority **[S1]**
- [ ] Board permission (view/edit per board) **[S2]**

### 2.5 Navigasi & Struktur **[DONE]**
- [x] Dashboard: daftar workspace
- [x] Sidebar per workspace: dokumen, board, "Shared with me", rename inline
- [x] Settings workspace: member + invite management (tabs members/invites)
- [x] Settings profil user
- [ ] Global search (dokumen/board lintas workspace) **[S1]**
- [ ] Star/pin item di sidebar **[S1]**
- [ ] Recent items **[S1]**
- [ ] Trash/soft-delete dengan masa retensi **[S1]**

### 2.6 Notifikasi & Feedback UI **[DONE]**
- [x] Toast notification (sukses/gagal/info), auto-dismiss
- [x] Empty state jelas (no workspace, no invitations, no history) — dengan CTA
- [x] Loading state semua async (skeleton loader)
- [x] Error state informatif + tombol Retry (dashboard, invites, history) — error TIDAK
  tampil sebagai empty state
- [x] 404 page + global error boundary + per-route error boundary
- [ ] Notification center (persisted) **[S1]**
- [ ] Email notification (invite, mention) **[S1]**

---

## 3. Sitemap / Routing (Next.js App Router) **[DONE]**

```
/                          → landing/redirect ke /login atau /dashboard
/login                     → halaman login
/register                  → halaman register
/invite/[token]            → terima/tolak invite via link
/invites                   → daftar pending invitation user
/dashboard                 → daftar workspace user
/w/[workspaceId]           → workspace home (empty state: pilih doc/board)
/w/[workspaceId]/doc/[docId]          → editor dokumen real-time
/w/[workspaceId]/doc/[docId]/history  → version history + restore
/w/[workspaceId]/board/[boardId]      → task board
/w/[workspaceId]/settings  → members + invites management
/settings/profile          → profil user
```

Layout:
- `(auth)` group untuk login/register
- AuthGuard (`web/components/auth-guard.tsx`) membungkus semua halaman terproteksi
- `w/[workspaceId]` layout: sidebar kiri (docs/boards/shared) + slot konten

---

## 4. Spesifikasi Teknis Mendalam

### 4.1 Database Schema (PostgreSQL) **[DONE — migrasi 00001..00008]**

Migrasi dikelola **goose** (embedded FS, auto-up saat startup server). Riwayat:

| Migrasi | Isi |
|---|---|
| 00001_init | users, refresh_tokens, workspaces, workspace_members, workspace_invites, documents, document_snapshots, document_events, boards, board_columns, tasks |
| 00002_boards | (melengkapi board) |
| 00003_invites | invite fields + token unique |
| 00004_invite_invited_by | invited_by FK |
| 00005_invite_indexes | index pendukung |
| 00006_fix_invites_email_type | normalisasi email |
| 00007_document_shares | **tabel baru** document_shares (sharing per-dokumen) |
| 00008_audit_fixes | drop duplicate indexes, FK snapshots/events `ON DELETE SET NULL`, pruning events runtime |

```sql
-- Inti (disimplifikasi; file asli di server/internal/migrations/*.sql)
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL,
    avatar_color TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,          -- hash, bukan token mentah
    expires_at TIMESTAMPTZ NOT NULL,
    revoked BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    owner_id UUID REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE workspace_members (
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner','editor','viewer')),
    joined_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);

CREATE TABLE workspace_invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('editor','viewer')),
    token TEXT UNIQUE NOT NULL,
    invited_by UUID REFERENCES users(id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted BOOLEAN DEFAULT FALSE
);

CREATE TABLE documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE document_shares (           -- [DONE] migrasi 00007
    document_id UUID REFERENCES documents(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    shared_by UUID REFERENCES users(id) ON DELETE SET NULL,
    permission TEXT NOT NULL CHECK (permission IN ('view','edit')),
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (document_id, user_id)
);

CREATE TABLE document_snapshots (
    id BIGSERIAL PRIMARY KEY,
    document_id UUID REFERENCES documents(id) ON DELETE CASCADE,
    state BYTEA NOT NULL,
    version INTEGER NOT NULL,            -- +1 per snapshot
    event_count INTEGER NOT NULL DEFAULT 0,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE document_events (
    id BIGSERIAL PRIMARY KEY,
    document_id UUID REFERENCES documents(id) ON DELETE CASCADE,
    update BYTEA NOT NULL,               -- raw Yjs update (idempotent)
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_document_events_snapshot_prune ON document_events (document_id, created_at);

CREATE TABLE boards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE board_columns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id UUID REFERENCES boards(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    position DOUBLE PRECISION NOT NULL
);

CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    column_id UUID REFERENCES board_columns(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    assignee_id UUID REFERENCES users(id),
    position DOUBLE PRECISION NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ DEFAULT now()
);
```

**Keputusan penting:**
- `document_events` tidak memakai `seq`/vector clock global — cukup `created_at` +
  idempotensi CRDT. Snapshot + replay dijamin konsisten tanpa koordinasi lintas tabel.
- **Pruning runtime** (`PruneEventsBeforeSnapshot`): setelah snapshot disimpan, event
  yang lebih tua dari snapshot terbaru dihapus — tabel events tidak tumbuh tanpa batas.
- **FK `ON DELETE SET NULL`** untuk created_by/shared_by: menghapus user tidak pernah
  gagal karena referensi audit trail.
- **Duplicate indexes dibuang** di migrasi 00008 (UNIQUE constraint sudah membuat index).

### 4.2 REST API Endpoints **[DONE]**

```
POST   /api/auth/register
POST   /api/auth/login
POST   /api/auth/refresh
POST   /api/auth/logout

GET    /api/workspaces
POST   /api/workspaces
GET    /api/workspaces/{id}
PATCH  /api/workspaces/{id}                      (rename)
DELETE /api/workspaces/{id}

GET    /api/workspaces/{id}/members
POST   /api/workspaces/{id}/invites
PATCH  /api/workspaces/{id}/members/{userId}     (ubah role)
DELETE /api/workspaces/{id}/members/{userId}
GET    /api/workspaces/{id}/invites              (daftar invite, owner-only)
POST   /api/invites/{token}/accept
POST   /api/invites/{token}/reject               [DONE]
GET    /invites/pending                          (daftar pending user)

GET    /api/workspaces/{id}/documents            (member = semua; non-member = shared)
POST   /api/workspaces/{id}/documents            (wajib member; IDOR-fixed)
GET    /api/documents/{id}
PATCH  /api/documents/{id}                       (rename; HasDocumentAccess)
DELETE /api/documents/{id}                       (hanya member)
GET    /api/documents/{id}/snapshots
POST   /api/documents/{id}/snapshots/{snapshotId}/restore   (ownership check)

POST   /api/documents/{id}/shares                [DONE] (share ke user)
GET    /api/documents/{id}/shares                [DONE] (list + canManage)
DELETE /api/documents/{id}/shares/{userId}       [DONE] (unshare)
GET    /api/documents/shared                     [DONE] (semua doc yang di-share ke saya)
GET    /api/users/by-email/{email}               [DONE] (lookup; rate-limited anti-enumeration)

GET    /api/workspaces/{id}/boards
POST   /api/workspaces/{id}/boards
GET    /api/boards/{id}
POST   /api/boards/{id}/columns
PATCH  /api/columns/{id}
DELETE /api/columns/{id}
POST   /api/columns/{id}/tasks
PATCH  /api/tasks/{id}                           (termasuk position + version check)
DELETE /api/tasks/{id}

GET    /api/users/me
PATCH  /api/users/me     (alias lama /users/me tetap didukung — backward-compat)
GET    /healthz
GET    /readyz
GET    /metrics                                    [S1 — placeholder]
```

**Aturan global:**
- Semua endpoint kecuali `/auth/*` dan `/invites/{token}/*` melewati `RequireAuth`.
- Semua akses workspace/dokumen melewati membership check (atau `HasDocumentAccess`
  untuk share) — **server-side, bukan klien**.
- Response error seragam: `{ "error": { "code": "...", "message": "..." } }` — lihat
  `httpapi/response.go` + `types.go`.
- Body limit 1 MB (`MaxBytesReader`) di semua decode.
- Rate limit: auth endpoints ketat (2/s burst 10), email lookup 5/s burst 20, umum 20/s
  burst 60.

### 4.3 WebSocket Protocol **[DONE]**

```
Endpoint: wss://host/ws/doc/{document_id}    → sync Yjs + awareness + role
Endpoint: wss://host/ws/board/{board_id}     → BOARD_EVENT JSON (live update board)
```

Frame top-level: `[1 byte message type][payload]`

| Byte | Nama | Arah | Keterangan |
|---|---|---|---|
| 0 | MsgSync | dua arah | sub-protocol sync Yjs |
| 1 | MsgAwareness | dua arah | presence (cursor, selection) |
| 2 | MsgAuth | client→server | auth tambahan (cadangan) |
| 3 | MsgQueryAwareness | client→server | minta state awareness |
| 5 | **MsgRole** | server→client | **role user** (owner/editor/viewer/view) — dipakai client render read-only |
| 6 | MsgPing | dua arah | heartbeat |
| 7 | MsgPong | dua arah | balas heartbeat |

Sync sub-protocol (byte payload pertama):

| Byte | Nama | Arah | Isi |
|---|---|---|---|
| 0 | SyncStep1 | dua arah | state vector |
| 1 | SyncStep2 | dua arah | full document update |
| 2 | Update | client→server / relay | incremental Yjs update |

**Filosofi relay (kritis):** server TIDAK memiliki library Yjs. Ia menyimpan:
- `doc.lastState` — full state terakhir yang di-write-back client (opaque bytes)
- `doc.replayEvents` — events dari DB (setelah snapshot terakhir) untuk client baru
- `doc.pendingEvents` — buffer update yang belum di-flush ke DB

Handshake:
1. Client connect → kirim `SyncStep1(stateVector)`.
2. Jika server punya `lastState` → balas `SyncStep2(lastState)` + replay events +
   **kirim `SyncStep1` balik** (minta state client — edit offline tidak hilang).
3. Jika tidak punya state → kirim `SyncStep1(kosong)` → client balas `SyncStep2(full)`.

**Enforcement read-only (IDOR-fixed):** `isReadOnly(role)` = `role == "viewer"` (workspace)
ATAU `role == "view"` (share permission). Viewer yang mengirim `Update`/`SyncStep2`
di-*drop* (tidak memutus koneksi — robust terhadap bug client). Role dikirim ke client
via MsgRole agar UI benar-benar read-only (editable=false, update remote tetap live).

**Heartbeat:** ping tiap 30s; tidak ada pong 10s → tutup koneksi. Client: backoff
1s→2s→4s→8s→max 30s + deteksi zombie (tidak ada data 75s → refresh token + reconnect).
Board: ping 30s + reload on reconnect.

**Slow consumer:** send buffer per-koneksi; buffer penuh → drop pesan (client akan
resync via heartbeat/step1) — satu koneksi lambat tidak memblokir yang lain.

### 4.4 Undo/Redo **[DONE]**

- `Y.UndoManager` di-scope per user (`trackedOrigins: [user.id]`).
- Test skenario wajib: A ketik → B ketik → A undo → hanya perubahan A hilang.
- Redo stack hilang saat refresh (trade-off disengaja; didokumentasikan).

### 4.5 Task Board Conflict Resolution **[DONE]**

- Fractional indexing: `position` float; insert tengah = rata-rata posisi tetangga
  (presisi `Math.round(*1e9)/1e9`).
- Optimistic concurrency: `UPDATE tasks SET position=?, version=version+1 WHERE id=? AND version=?`.
  Affected rows = 0 → refetch + reconcile (retry sekali dengan posisi relatif sama).
- Hasil final di-broadcast via `BOARD_EVENT` — semua client konsisten.

### 4.6 Persistence & Compaction **[DONE]**

Pipeline:
```
Update masuk (WS)
  → doc.AddPendingEvent (buffer in-memory, per-document)
  → worker (interval 5s flush + 3m snapshot):
      a. Flush pendingEvents → document_events (batch insert, transaksional)
      b. Jika state fresh → SaveSnapshot(state, eventCount) → PruneEventsBeforeSnapshot
      c. Jika needsWriteBack → kirim SyncStep1 ke client (minta full state baru)
  → doc.SetState(state) (dari client) → stateFresh = (eventCount == 0)  [fix C3]
```

Aturan anti-hilang-data:
- `SetState` TIDAK pernah mengosongkan `pendingEvents` (fix C3 — update in-flight
  tidak boleh hilang; duplikasi aman karena CRDT idempotent).
- Gagal insert DB → `RestorePendingEvents` → dicoba lagi di siklus berikutnya.
- Shutdown: `FlushNow()` sinkron sebelum proses keluar + store `Close()`.
- Memory: `MaybeEvict` (koneksi habis + bersih) dan `EvictStale` (ticker 5 menit —
  dokumen yang ditinggalkan tetap bisa di-evict karena state sudah di DB).

Load dokumen: snapshot terakhir + `LoadEventsSince(snapshot.created_at)` + replay ke
client baru (per-koneksi, tidak pernah di-clear global — fix M1).

### 4.7 Keamanan (Security Posture) **[DONE]**

| Lapisan | Implementasi |
|---|---|
| Transport | HSTS (saat TLS), CORS ketat (`CORS_ORIGIN`) |
| Headers | `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`, `X-XSS-Protection`, `Permissions-Policy`, CSP (`default-src 'self'`, `connect-src 'self' ws: wss:`, `frame-ancestors 'none'`) |
| Auth | JWT access (15m) + refresh token hash di DB, httpOnly cookie, silent refresh |
| Otorisasi | requireMember / HasDocumentAccess / role check per endpoint + WS read-only enforcement |
| Rate limit | token bucket per IP: auth 2/s burst 10, email 5/s burst 20, umum 20/s burst 60; header `Retry-After` |
| Payload | body limit 1 MB; WS message validation (varint decode, reject malformed) |
| Secret hygiene | `JWT_SECRET` ≥32 char di prod; DSN di-log dengan password di-mask; token invite TIDAK bocor via list API yang salah scope |
| Anti-enumeration | `/api/users/by-email` rate-limited; error tidak membedakan email terdaftar/tidak |
| Logging | request ID per request; query string TIDAK di-log (token WS ada di query) |
| XSS | semua teks user di-render via React (escape otomatis); CSP membatasi script |

**TODO keamanan horizon berikutnya:** argon2id (saat ini bcrypt), audit log
immutable, `SECURITY.txt`, CSP dengan nonce, rate limit per-user (bukan hanya per-IP),
2FA, SSO, e2ee.

### 4.8 Observability **[DONE → S1]**

- `slog` JSON terstruktur: setiap request punya `request_id`, `method`, `path`, `status`,
  `duration_ms`, `remote`. Log level info/warn/error.
- Health endpoint: `GET /healthz` (liveness, tidak bocor detail error internal),
  `GET /readyz` (readiness — dipakai orchestrator untuk rolling deploy).
- **TODO [S1]:** `/metrics` Prometheus (counter request, histogram latency, gauge
  WS connections per doc, pending events count), OTel trace (WS handshake,
  flush worker, snapshot), structured audit log.

### 4.9 Multi-Instance / Scale-Out (Arsitektur Horizon 1→2)

Skala saat ini: **single-instance, in-memory store + Postgres** — benar untuk H1.

Saat butuh multi-instance:
1. Store per-document dipindah ke pola **separation**: state "panas" tetap in-memory,
   tapi authority pindah ke Postgres (events adalah source of truth).
2. Broadcast lintas node via **Redis Pub/Sub** (`doc:{id}:events` channel) — setiap
   node subscribe; client tetap terhubung ke satu node (sticky).
3. `PulseWSProvider` di client TIDAK berubah — transisi transparan.
4. Region scale (H2): shard per workspace/region; read replica untuk history;
   snapshot/compaction terjadwal di satu node (leader election ringan via Postgres
   advisory lock).

---

## 5. Sistem Desain UI (Non-Negotiable) **[DONE]**

### 5.1 Tipografi
- Native font stack: `-apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif`.
- BUKAN font display startup. Rasa: aplikasi kantor/enterprise.
- Base 14–16px; heading hierarchy jelas, tidak berlebihan.

### 5.2 Warna & Layout
- Neutral base (white/off-white), abu-abu border/secondary, SATU accent (indigo/blue).
- Tanpa gradient/glassmorphism/shadow berlebihan. Spacing scale kelipatan 4px.
- Border-radius konsisten (6–8px).

### 5.3 Komponen & State
- Semua interaktif punya state: default/hover/active/disabled/loading.
- Skeleton loader untuk async; empty state dengan CTA; toast auto-dismiss.

### 5.4 Responsif & Aksesibilitas
- Desktop-first (tablet best-effort; mobile bukan prioritas).
- WCAG AA contrast; focus state terlihat; tab order jelas.
- Shortcut editor: Ctrl/Cmd+B/I/Z/Shift+Z.
- Modal board: `role="dialog"` + `aria-modal` + Escape-to-close **[DONE]**.

---

## 6. Non-Functional Requirements (SLA-Enabled)

| Area | Target | Verifikasi |
|---|---|---|
| Uptime (H1) | 99.9% (≈8.7h downtime/tahun) | health/ready + load balancer |
| Uptime (H2) | 99.95% multi-region | failover otomatis |
| RTO | ≤15 menit (restore dari snapshot + events) | DR drill terjadwal |
| RPO | 0 — tidak ada data loss (buffer + flush + FlushNow) | tes kill -9 saat event pending |
| Latency broadcast | p50 < 100ms node-lokal | E2E test 2 browser |
| Throughput WS | 1k CCU/node tanpa degradasi (H1) | load test k6 |
| Body size | ≤1 MB/request, ≤64 KB/WS frame | unit + integration |
| Concurrency board | 2 user drag task sama → tidak ada data hilang | E2E + version check |
| Browser | Chrome & Firefox terbaru (target utama) | Playwright |
| Konfigurasi | 100% env var + `.env.example` | audit kode |

---

## 7. Testing Wajib

### 7.1 Unit Test (Go) **[DONE — coverage kritis]**

Sudah ada:
- `auth/jwt_test.go`: issue/verify, expired token, signature invalid.
- `boards/repo_test.go`: fractional indexing, CRUD (butuh DB live).
- `yws/store_test.go`: **fix C3** (SetState tidak clear pendingEvents), **fix M1**
  (GetReplayEvents tidak clear, defensive copy), **fix M2** (EvictStale), pending
  event clear/restore, MaybeEvict.
- `yws/processor_test.go`: isReadOnly (viewer + "view"), viewer cannot Update /
  cannot SyncStep2, editor can Update, EncodeRoleMessage roundtrip.
- `httpapi/enterprise_middleware_test.go`: rate limiter (burst/refill/middleware 429 +
  Retry-After), security headers lengkap, request ID (generate + propagate).

Wajib ditambah:
- [ ] Fractional indexing ekstrem: 10k insert tengah → presisi tetap (S1)
- [ ] Optimistic concurrency task: version stale → error code 409 (S1)
- [ ] Password strength validator (S1)
- [ ] Refresh token rotation + reuse detection (S1)

### 7.2 Integration Test (Go, butuh DB/Redis)
- [ ] WS handshake penuh: connect → SyncStep1/2 → Update → flush ke Postgres (S1)
- [ ] Replay setelah restart: snapshot + events → state client benar (S1)
- [ ] Role enforcement: viewer kirim Update → ditolak server (S1)
- [ ] FlushNow saat shutdown tidak kehilangan data (S1)

### 7.3 E2E (Playwright) — skenario wajib
1. Dua browser (user berbeda) buka dokumen sama → ketik → muncul di context lain <1s.
2. Selective undo (§4.4) terverifikasi otomatis.
3. Dua context drag task bersamaan → tidak ada task hilang/duplikat.
4. Disconnect WS → UI "reconnecting" → reconnect → state ter-sync.
5. **Offline edit**: putus koneksi → ketik → konek lagi → edit TIDAK hilang (fix C2).
6. Viewer via share "view" → editor read-only (banner + tidak bisa ketik).
7. Unauthorized user akses `/w/{id}/doc/{docId}` → ditolak.

---

## 8. Struktur Folder **[DONE]**

```
pulse/
├── docker-compose.yml
├── .env.example
├── docs/task.md                 ← dokumen ini
├── bin/                         ← build artifact (pulse.exe)
├── server/                      ← Go backend
│   ├── cmd/pulse/main.go        ← entrypoint (config → DB → migrate → worker → server)
│   ├── internal/
│   │   ├── auth/                ← jwt, password, refresh token, wsauth
│   │   ├── boards/              ← board/column/task repo
│   │   ├── config/              ← env config (12-factor)
│   │   ├── db/                  ← pool + goose migrate
│   │   ├── documents/           ← doc repo + snapshot repo + shares + prune
│   │   ├── health/              ← healthz/readyz
│   │   ├── httpapi/             ← router, middleware, handlers, enterprise_middleware
│   │   ├── migrations/          ← goose SQL 00001..00008 (embedded)
│   │   ├── models/              ← struct model
│   │   ├── persistence/         ← background worker (flush + snapshot + evict)
│   │   ├── users/               ← user repo
│   │   ├── workspaces/          ← workspace/member/invite repo
│   │   ├── ycodec/              ← varint/varbuffer encode-decode (protocol)
│   │   └── yws/                 ← store, conn, processor, handler, board_handler
│   └── go.mod
└── web/                         ← Next.js frontend
    ├── app/                     ← App Router (lihat §3)
    ├── components/              ← auth-guard, invite-notifications, share-document-modal,
    │                              doc-sidebar-item
    ├── lib/                     ← api-client, yjs-provider (PulseWSProvider)
    ├── store/                   ← zustand auth store
    └── package.json
```

---

## 9. Dokumentasi Wajib (README.md) **[DONE — refresh terjadwal]**

1. Arsitektur (diagram + per-komponen) — termasuk keputusan: CRDT vs OT, fractional
   indexing, undo per-origin, relay strategy tanpa library Yjs di server.
2. Cara run lokal: `docker-compose up` (Postgres+Redis), `cd server && go run`,
   `cd web && npm run dev`; migrasi auto-apply.
3. **Catatan setup DB**: reset database = drop + create + jalankan server sekali
   (migrasi auto-apply); creds default di `.env`.
4. Known limitations (jujur): redo hilang saat refresh, presence downsampling,
   single-instance WS (multi-instance = roadmap S1), floating point fractional index
   (bukan string lexicographic).
5. API reference (bisa di-generate dari §4.2).
6. Security postur + cara melaporkan kerentanan.

---

## 10. Aturan Kerja untuk Agent

1. Kerjakan sesuai fase §12, jangan lompat; **setiap fase selesai → jalankan test
   relevan → tunjukkan hasil → baru lanjut**.
2. Komentar kode menjelaskan **rationale** (kenapa), bukan "apa" — khususnya
   conflict resolution, undo/redo, dan strategi relay.
3. Jangan ganti tech stack (§1) atau prioritas fitur (§2) tanpa diskusi.
4. Trade-off signifikan → laporkan & tunggu keputusan. Jangan diam-diam menyederhanakan.
5. **Defense-in-depth**: apapun yang bisa dicek server, dicek server. UI hanyalah lapisan.
6. Kode baru wajib: lint hijau (backend: `go vet ./...`, frontend: `npx eslint .`),
   typecheck hijau (`npx tsc --noEmit`), test hijau (`go test ./...`).
7. Jangan menulis komentar kosong/templat; jangan meninggalkan TODO yang tidak ada
   owner; setiap TODO di kode wajib punya issue/dokumen referensi.
8. **Verifikasi berulang**: setelah batch perubahan besar, jalankan verifikasi 100x
   (build+vet+test backend, tsc+lint frontend) sampai user menyuruh berhenti.

---

## 11. Kriteria "Selesai" untuk Seluruh Project

- [ ] Dua user berbeda login, buka dokumen sama, edit bersamaan real-time (E2E #1)
- [ ] Kursor live dengan warna & nama per user
- [ ] Selective undo terbukti benar (E2E #2)
- [ ] Restart/refresh server tidak menghilangkan data (integration + RPO=0 drill)
- [ ] Version history dilihat & di-restore
- [ ] Board DnD multi-user tanpa data hilang (E2E #3)
- [ ] Role viewer & share-"view" benar-benar read-only di server (integration)
- [ ] Status koneksi terlihat jelas; reconnect otomatis; **edit offline tidak hilang**
- [ ] Error/empty/loading state benar di semua halaman; 404 + error boundary ada
- [ ] Semua test §7 lulus; lint & typecheck bersih
- [ ] README lengkap §9; `.env.example` sinkron
- [ ] Database bersih ter-migrasi (00001..00008) tanpa error

---

## 12. Execution Plan (Roadmap 20 Tahun)

### Fase 0 — Fondasi & Baseline **[DONE]**
- [x] Schema DB + migrasi goose (1–8), docker-compose
- [x] Auth lengkap (register/login/refresh/logout, httpOnly cookie, bcrypt)
- [x] Health/ready endpoint
- [x] Config 12-factor + `.env.example`

### Fase 1 — Sync dasar **[DONE]**
- [x] WS single-instance, SyncStep1/2, Update relay, ProseMirror + PulseWSProvider
- [x] Handshake dua arah (fix C2), replay per-koneksi (fix M1)

### Fase 2 — Presence **[DONE]**
- [x] Awareness protocol, live cursor, avatar list, batching 50ms + client throttle

### Fase 3 — Persistence **[DONE]**
- [x] Snapshot + event log, replay saat load, background worker (flush 5s/snapshot 3m)
- [x] Pruning events, FlushNow graceful shutdown, eviction (M2)

### Fase 4 — Version history & undo/redo **[DONE]**
- [x] Timeline, restore dengan ownership check
- [x] Undo/redo per-origin + skenario selective undo

### Fase 5 — Auth hardening & sharing **[DONE]**
- [x] Role enforcement server-side (workspace + share "view")
- [x] Invite flow lengkap (link, pending, accept/reject, cancel)
- [x] **Document sharing per-dokumen** (share/unshare/list/shared-with-me/by-email)

### Fase 6 — Task board **[DONE]**
- [x] Schema, fractional indexing, drag-drop, optimistic concurrency + version check
- [x] WS BOARD_EVENT real-time

### Fase 7 — Connection resilience & UX **[DONE]**
- [x] Reconnect + backoff, status indicator, heartbeat, zombie detection
- [x] Error/empty/loading states, 404, error boundaries, toast
- [x] Board modal aksesibilitas, read-only viewer UX

### Fase 8 — Enterprise hardening **[DONE]**
- [x] Security headers + CSP + request ID + structured logging
- [x] Rate limiting (auth + umum + email lookup)
- [x] Body size limit, DSN masking, health tanpa bocor detail
- [x] Dead deps cleanup + ESLint config ketat (0 warning)
- [x] Unit tests untuk semua fix kritis (store, processor, middleware)

### Fase 9 — Scale-Out (Horizon 1, S1)
- [ ] Redis Pub/Sub broadcast lintas node; sticky routing
- [ ] Leader election (advisory lock) untuk snapshot/compaction
- [ ] `/metrics` Prometheus + OTel traces
- [ ] Audit log (siapa/apa/kapan) + user-facing activity
- [ ] Soft-delete/trash + retensi
- [ ] Notifikasi center + email (invite/mention)

### Fase 10 — Compliance & Enterprise (Horizon 2, S2)
- [ ] SSO/OIDC + SCIM provisioning, 2FA
- [ ] RBAC granular (per-board/per-doc permission), SAML
- [ ] Data residency & backup otomatis, DR drill RTO 15m
- [ ] Load test k6 (1k CCU/node), tuning pool & eviction
- [ ] PWA offline-first dokumen (IndexedDB + queue)

### Fase 11 — Evolve (Horizon 3, S3)
- [ ] AI copilot (ringkasan/tulis lanjut/deteksi konflik semantic)
- [ ] E2EE opsional (client-side key, server blind)
- [ ] CRDT-as-a-Service / serverless transport (Node/polyfill transport swap via provider)
- [ ] Air-gap / on-prem bundle (single binary + embedded assets)

---

## 13. Keputusan Arsitektur & Trade-off (ADRs Ringkas)

| # | Keputusan | Alternatif ditolak | Alasan |
|---|---|---|---|
| ADR-1 | CRDT (Yjs) bukan OT | OT (ShareDB, ot.js) | OT butuh server rekanan state & serialisasi; CRDT konvergen dengan transport unreliable |
| ADR-2 | Relay tanpa library Yjs di server | Yjs di Go (ygotalk) | Ekosistem Go Yjs tidak matang; relay + persistence cukup untuk kebutuhan; client tetap source of truth CRDT |
| ADR-3 | Snapshot + event log | Full state per save / event only | Snapshot mempercepat load; event log menjaga RPO=0 |
| ADR-4 | Fractional indexing float untuk board | Yjs Array / string lexicographic | Sederhana, cocok untuk pola pindah-dan-sisip; presisi di-manage (1e9 rounding) |
| ADR-5 | Refresh token hash + revoke di DB | JWT-only / stateless refresh | Revocable & rotasi; biaya lookup kecil |
| ADR-6 | Handshake dua arah (server minta state) | One-way push | Edit offline tidak boleh hilang (C2) — server minta state balik |
| ADR-7 | Pending events tidak pernah di-clear oleh SetState | Clear-on-set | Race C3: update in-flight hilang; duplikasi CRDT aman |
| ADR-8 | Per-IP rate limiting | Per-user | Tanpa Redis, per-IP di single node sederhana; naik ke per-user saat Redis dipakai lintas node |
| ADR-9 | Env var saja (12-factor) | Config file | Portabilitas env; secret tidak di repo |
| ADR-10 | Migrasi auto-apply saat startup | Migrasi manual/CLI | Simpel untuk dev & small prod; di prod bisa diganti goose CLI di pipeline CI |

---

## 14. Checklist Verifikasi Rilis (Release Gate)

Sebelum release baru:
1. [ ] `go build ./... && go vet ./... && go test ./...` hijau
2. [ ] `npx tsc --noEmit` & `npx eslint .` hijau (0 warning)
3. [ ] `npx next build` sukses (production build)
4. [ ] Migrasi 00001..00008 ter-apply di DB bersih tanpa error
5. [ ] `pulse.exe` start → log: `migrations applied`, `persistence worker started`, `server starting`
6. [ ] Health `/healthz` & `/readyz` 200
7. [ ] Smoke test: register → login → buat workspace → buat doc → edit 2 browser → buat board → drag task
8. [ ] Restart server → data tetap ada (RPO=0 spot-check)

---

*Dokumen ini final untuk memulai/continue development. Update hanya lewat diskusi eksplisit.*
*Revisi: v4.0 — 2026-08-14 — enterprise-grade, 20-tahun horizon, mencerminkan baseline repo saat ini.*
