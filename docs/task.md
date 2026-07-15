# PULSE — MASTER BUILD PROMPT (Lengkap & Self-Contained)

> Dokumen ini adalah brief tunggal, lengkap, dan final untuk membangun Pulse dari nol sampai deployable. Copy seluruh isi ke Claude Code. Dokumen ini dirancang untuk berdiri sendiri — tapi kalau ada file `pulse-technical-spec.md` dari sesi sebelumnya, upload juga sebagai referensi tambahan untuk rationale arsitektur.

---

## 0. Ringkasan Eksekutif

Bangun **Pulse**: real-time collaborative workspace dengan dokumen kolaboratif (rich text, multi-cursor live editing), task board dengan drag-and-drop multi-user, live presence, version history, dan undo/redo yang benar secara multiplayer. Target: portfolio-grade, kualitas produksi kecil — bukan tutorial, bukan prototype yang rapuh. Setiap fitur di bawah ini **wajib** ada kecuali ditandai eksplisit sebagai opsional/stretch.

Kerjakan bertahap sesuai §12 (Execution Plan). Di setiap fase, tunjukkan hasil kerja (running demo/test) sebelum lanjut. Kalau ada keputusan arsitektur yang ambigu, tanya dulu — jangan asumsi sepihak untuk hal yang berdampak luas.

---

## 1. Tech Stack (Final, Tidak Boleh Diganti Tanpa Diskusi)

| Layer | Teknologi |
|---|---|
| Backend real-time & API | Go 1.22+ (`gorilla/websocket` atau `nhooyr.io/websocket`, `chi` atau `gin` untuk REST) |
| CRDT | Yjs (`yjs`, `y-protocols/sync`, `y-protocols/awareness`) di client; Go server sebagai relay + persistence |
| Database | PostgreSQL 15+ |
| Cache & Pub/Sub | Redis 7+ |
| Frontend framework | Next.js 14+ (App Router), TypeScript strict mode |
| Editor | ProseMirror + `y-prosemirror` |
| State management (non-CRDT/local UI) | Zustand |
| Styling | Tailwind CSS (utility classes, tanpa komponen UI library yang berat seperti MUI) |
| Auth | JWT access token + refresh token (httpOnly cookie) |
| Containerization | Docker + docker-compose untuk local dev (Postgres, Redis, backend, frontend) |
| Testing | Go: `testing` + `testify`. Frontend: Vitest/Jest untuk unit, Playwright untuk E2E |

---

## 2. Daftar Fitur Lengkap (Semua Wajib Kecuali Ditandai Opsional)

### 2.1 Auth & User Management
- Register (email, password, display name), validasi password strength minimal
- Login, logout
- Refresh token flow otomatis (silent refresh di frontend saat access token expired)
- Password hashing (bcrypt/argon2)
- Update profil (nama, avatar color)
- *(Opsional)* Reset password via email

### 2.2 Workspace & Membership
- Create workspace
- Invite member (via email atau invite link dengan token)
- List member & role (owner/editor/viewer)
- Ubah role member (hanya owner)
- Remove member
- Switch antar workspace (kalau user punya lebih dari satu)

### 2.3 Document Collaboration (Tier 1 — Core)
- Create/rename/delete dokumen dalam workspace
- Rich-text block-based editing (heading, paragraph, bullet list, numbered list, bold/italic/underline, code block) — ala Notion sederhana
- Real-time multi-user editing, perubahan terlihat instan di semua client terhubung
- Live cursor per user (warna unik per user, label nama saat hover/dekat kursor)
- Avatar presence list (siapa saja sedang membuka dokumen ini sekarang)
- Selective undo/redo (per-user origin, lihat §4.4)
- Auto-save (tidak ada tombol "save" manual — semua tersimpan otomatis via event log)
- Version history: timeline snapshot, preview versi lama, restore ke versi tertentu
- Read-only mode untuk role viewer (enforced di server, bukan cuma UI)
- Indikator status koneksi (connected / reconnecting / offline) — **wajib**, karena real-time app tanpa indikator ini terasa tidak profesional dan membingungkan user saat koneksi putus
- Reconnection otomatis dengan exponential backoff, dan re-sync state begitu koneksi pulih

### 2.4 Task Board (Tier 2)
- Create board dalam workspace
- Create/rename/delete column
- Create/edit/delete task (title, opsional: deskripsi singkat, assignee)
- Drag-and-drop task antar kolom dan reorder dalam kolom, multi-user tanpa konflik (fractional indexing)
- Real-time reflect perubahan board ke semua user yang membuka board yang sama
- Optimistic concurrency handling saat 2 user drag task yang sama bersamaan

### 2.5 Navigasi & Struktur Aplikasi
- Dashboard: daftar workspace milik user
- Sidebar per workspace: daftar dokumen & board
- Halaman search sederhana (cari dokumen/board berdasarkan judul dalam satu workspace) — *(opsional tapi direkomendasikan)*
- Settings halaman: profil user, manajemen member workspace

### 2.6 Notifikasi & Feedback UI (wajib, sering diabaikan)
- Toast notification untuk aksi (berhasil invite, gagal simpan, dll)
- Empty state yang jelas (workspace belum ada dokumen, board belum ada task)
- Loading state untuk semua async action (skeleton loader, bukan blank screen)
- Error state yang informatif (bukan cuma "Something went wrong") saat WS gagal connect atau API error

---

## 3. Sitemap / Routing (Next.js App Router)

```
/                          -> landing/redirect ke /login atau /dashboard
/login
/register
/dashboard                 -> daftar workspace user
/w/[workspaceId]            -> workspace home (daftar dokumen & board)
/w/[workspaceId]/doc/[docId]        -> editor dokumen real-time
/w/[workspaceId]/doc/[docId]/history -> version history dokumen
/w/[workspaceId]/board/[boardId]     -> task board
/w/[workspaceId]/settings   -> member management, invite
/settings/profile           -> profil user
```

---

## 4. Spesifikasi Teknis Mendalam

### 4.1 Database Schema (PostgreSQL)

```sql
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
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    owner_id UUID REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE workspace_members (
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
    joined_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);

CREATE TABLE workspace_invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('editor', 'viewer')),
    token TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted BOOLEAN DEFAULT FALSE
);

CREATE TABLE documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE document_snapshots (
    id BIGSERIAL PRIMARY KEY,
    document_id UUID REFERENCES documents(id) ON DELETE CASCADE,
    state BYTEA NOT NULL,
    vector_clock BYTEA NOT NULL,
    event_seq_at_snapshot INTEGER NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE document_events (
    id BIGSERIAL PRIMARY KEY,
    document_id UUID REFERENCES documents(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id),
    update_bin BYTEA NOT NULL,
    seq INTEGER NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_doc_events_doc_seq ON document_events(document_id, seq);

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

### 4.2 REST API Endpoints

```
POST   /api/auth/register
POST   /api/auth/login
POST   /api/auth/refresh
POST   /api/auth/logout

GET    /api/workspaces
POST   /api/workspaces
GET    /api/workspaces/:id
PATCH  /api/workspaces/:id
DELETE /api/workspaces/:id

GET    /api/workspaces/:id/members
POST   /api/workspaces/:id/invites
POST   /api/invites/:token/accept
PATCH  /api/workspaces/:id/members/:userId    (ubah role)
DELETE /api/workspaces/:id/members/:userId

GET    /api/workspaces/:id/documents
POST   /api/workspaces/:id/documents
GET    /api/documents/:id
PATCH  /api/documents/:id            (rename)
DELETE /api/documents/:id
GET    /api/documents/:id/snapshots  (version history list)
POST   /api/documents/:id/snapshots/:snapshotId/restore

GET    /api/workspaces/:id/boards
POST   /api/workspaces/:id/boards
GET    /api/boards/:id
POST   /api/boards/:id/columns
PATCH  /api/columns/:id
DELETE /api/columns/:id
POST   /api/columns/:id/tasks
PATCH  /api/tasks/:id                (termasuk update position, dengan version check)
DELETE /api/tasks/:id

GET    /api/users/me
PATCH  /api/users/me
```

Semua endpoint (kecuali `/auth/*` dan `/invites/:token/accept`) wajib melalui middleware auth + workspace membership check.

### 4.3 WebSocket Protocol

```
Endpoint: wss://host/ws/doc/{document_id}   -> untuk document sync
Endpoint: wss://host/ws/board/{board_id}    -> untuk board live update

Frame format: [1 byte message type][payload]
0x01 SYNC_STEP_1      client -> server: state vector
0x02 SYNC_STEP_2      server -> client: diff update
0x03 UPDATE           dua arah: incremental Yjs update
0x04 AWARENESS        dua arah: presence (cursor, selection, status)
0x05 AUTH             client -> server: JWT, dikirim sebagai first message
0x06 PING / 0x07 PONG heartbeat
0x08 BOARD_EVENT      board channel: task moved/created/updated/deleted (JSON payload, bukan Yjs)
```

Aturan koneksi:
- Auth via first message setelah `onopen`, bukan lewat URL query.
- Server reject `UPDATE`/`BOARD_EVENT` dari koneksi dengan role `viewer`.
- Heartbeat tiap 30 detik; kalau tidak ada pong dalam 10 detik, anggap koneksi mati dan tutup dari sisi server.
- Client: reconnect dengan exponential backoff (1s, 2s, 4s, 8s, maks 30s), tampilkan status di UI selama proses ini (lihat §2.3).

### 4.4 Undo/Redo

- `Y.UndoManager` di-scope per user (per origin field pada setiap transaction Yjs).
- Test case wajib: User A ketik teks → User B ketik teks setelahnya → User A undo → hanya perubahan User A yang hilang, perubahan User B tetap utuh.
- Redo stack client-side, hilang saat refresh (trade-off yang disengaja, dokumentasikan di README).

### 4.5 Task Board Conflict Resolution

- Fractional indexing untuk `position` (float) — insert di antara dua task dengan mengambil rata-rata posisi tetangganya.
- Update `position` dan `version` bersamaan: `UPDATE tasks SET position=?, version=version+1 WHERE id=? AND version=?`. Jika affected rows = 0, re-fetch state terbaru dan reconcile intent user (retry sekali dengan posisi relatif yang sama).
- Broadcast hasil final ke semua client via `BOARD_EVENT`, bukan optimistic lokal saja.

### 4.6 Persistence & Compaction

- Setiap `UPDATE` masuk ke `doc:{id}:pending` di Redis, di-flush batched ke `document_events` (tiap 500ms atau 50 update, mana lebih dulu).
- Snapshot baru dibuat tiap 100 event atau tiap 5 menit aktif, mana lebih dulu tercapai — simpan di `document_snapshots` dengan `event_seq_at_snapshot` supaya replay tahu mulai dari mana.
- Load dokumen: ambil snapshot terakhir + replay `document_events` dengan `seq > event_seq_at_snapshot`.

---

## 5. Sistem Desain UI (Non-Negotiable)

### 5.1 Tipografi
- **Font: sans-serif standar/umum, BUKAN font display/dekoratif.** Gunakan native font stack: `font-family: -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;` — ini membuat user Windows melihat Segoe UI (visual serumpun dengan Calibri), user Mac melihat San Francisco, tanpa perlu font-loading tambahan.
- Kalau ingin lebih dekat ke tampilan Calibri secara spesifik: gunakan **Carlito** dari Google Fonts (metric-compatible, open-source, legal untuk web — Calibri asli tidak boleh di-embed di web tanpa lisensi Microsoft yang sesuai).
- **Jangan** pakai font display bergaya startup (Poppins, Space Grotesk, Sora, dll). Target rasa visual: profesional, netral, seperti aplikasi kantor/enterprise, bukan landing page produk konsumen.
- Ukuran: base 14–16px untuk body, hierarchy heading jelas tapi tidak berlebihan (h1/h2/h3 dengan jump size wajar, bukan drastis).

### 5.2 Warna & Layout
- Palet netral: putih/off-white sebagai base, abu-abu untuk border/secondary text, satu accent color (pilih satu warna profesional seperti biru atau indigo) untuk CTA dan elemen interaktif.
- Hindari gradient, glassmorphism, atau shadow berlebihan.
- Consistent spacing scale (misal kelipatan 4px: 4/8/12/16/24/32).
- Border-radius konsisten di seluruh komponen (pilih satu nilai, misal 6px atau 8px, pakai di semua tempat).

### 5.3 Komponen & State
- Setiap komponen interaktif punya state: default, hover, active, disabled, loading.
- Skeleton loader untuk semua data async (bukan spinner generik di tengah layar kosong).
- Empty state dengan ilustrasi sederhana/ikon + CTA jelas ("Belum ada dokumen. Buat dokumen pertama.").
- Toast notification di pojok (auto-dismiss, dengan opsi dismiss manual).

### 5.4 Responsif & Aksesibilitas
- Layout responsif minimal untuk desktop dan tablet (mobile boleh best-effort, bukan prioritas untuk app kolaboratif seperti ini).
- Kontras warna teks memenuhi WCAG AA minimal.
- Semua elemen interaktif bisa diakses via keyboard (tab order jelas, focus state terlihat).
- Keyboard shortcut dasar di editor: Ctrl/Cmd+B (bold), Ctrl/Cmd+I (italic), Ctrl/Cmd+Z (undo), Ctrl/Cmd+Shift+Z (redo).

---

## 6. Non-Functional Requirements

- **Performance:** presence broadcast di-throttle (client debounce ~20 update/detik, server batch window 50ms) — dijelaskan alasannya di README.
- **Security:** validasi ukuran payload WS (maks 1MB), rate limiting per koneksi, sanitasi update dari client, CORS ketat, refresh token revocable via tabel `refresh_tokens`.
- **Logging:** structured logging di Go server (level: info/warn/error), minimal log untuk connection lifecycle, auth failure, dan flush error ke Postgres.
- **Browser support:** Chrome & Firefox versi terbaru sebagai target utama (cukup untuk demo portfolio).
- **Konfigurasi:** semua kredensial (DB URL, Redis URL, JWT secret) lewat environment variable, sediakan `.env.example`.

---

## 7. Testing Wajib

### 7.1 Unit Test (Go)
- Fractional indexing: insert di tengah, insert di ujung, insert berulang (pastikan presisi float tidak habis dalam skenario wajar).
- JWT issuance & verification, termasuk kasus expired token.
- Optimistic concurrency check pada update task (versi lama vs baru).

### 7.2 Integration Test
- Flow: buka WS connection → auth → sync_step_1/2 → kirim update → verifikasi tersimpan di Postgres setelah flush.
- Role enforcement: koneksi dengan role viewer mengirim `UPDATE`, harus di-reject oleh server.

### 7.3 E2E Test (Playwright) — skenario wajib
1. Dua browser context login sebagai user berbeda, buka dokumen sama, ketik di satu context, verifikasi teks muncul di context lain dalam batas waktu wajar (misal <1 detik).
2. Skenario selective undo (§4.4) dijalankan dan diverifikasi otomatis.
3. Drag-and-drop task dari dua context bersamaan, verifikasi tidak ada task yang hilang/terduplikasi.
4. Simulasi disconnect (matikan WS), verifikasi UI menampilkan status "reconnecting", lalu reconnect otomatis dan state ter-sync ulang.

---

## 8. Struktur Folder (Contoh)

```
pulse/
├── docker-compose.yml
├── .env.example
├── server/                    # Go backend
│   ├── cmd/
│   ├── internal/
│   │   ├── auth/
│   │   ├── ws/
│   │   ├── persistence/
│   │   ├── board/
│   │   └── db/
│   └── go.mod
├── web/                        # Next.js frontend
│   ├── app/
│   │   ├── (auth)/
│   │   ├── dashboard/
│   │   ├── w/[workspaceId]/
│   │   └── settings/
│   ├── components/
│   ├── lib/
│   │   ├── yjs-provider.ts     # custom WS provider di atas y-protocols
│   │   └── api-client.ts
│   └── package.json
└── README.md
```

---

## 9. Dokumentasi yang Wajib Dihasilkan (README.md)

1. Arsitektur (diagram + penjelasan tiap komponen)
2. Cara run lokal (docker-compose up, migration, seed data)
3. Penjelasan keputusan desain penting: kenapa CRDT (bukan OT), kenapa fractional indexing untuk board (bukan Yjs Array), desain undo/redo per-origin
4. Known limitations (jujur, eksplisit): redo stack hilang saat refresh, presence downsampling di room besar (kalau diimplementasi), dsb.
5. Struktur API (bisa generate dari §4.2)

---

## 10. Aturan Kerja untuk Agent

- Kerjakan sesuai urutan fase di §12, jangan lompat.
- Setiap fase selesai: jalankan test yang relevan, tunjukkan hasilnya, baru lanjut.
- Tulis komentar kode yang menjelaskan **rationale**, khususnya di bagian conflict resolution dan undo/redo — bagian ini yang akan dijelaskan ke interviewer nantinya.
- Jangan mengganti tech stack (§1) atau melompati prioritas fitur (§2) tanpa mendiskusikannya dulu.
- Kalau menemukan trade-off signifikan (misal masalah performa yang butuh simplifikasi), laporkan dan tunggu keputusan, jangan diam-diam disederhanakan.

---

## 11. Kriteria "Selesai" untuk Seluruh Project

Project dianggap selesai kalau semua ini benar:
- [ ] Dua user berbeda bisa login, buka dokumen sama, edit bersamaan real-time
- [ ] Kursor live terlihat dengan warna & nama per user
- [ ] Undo/redo selective terbukti benar (test case §4.4 lulus)
- [ ] Refresh/restart server tidak menghilangkan data dokumen
- [ ] Version history bisa dilihat dan di-restore
- [ ] Task board drag-and-drop multi-user tidak menghasilkan data hilang/corrupt
- [ ] Role viewer benar-benar read-only (server-enforced)
- [ ] Status koneksi (connected/reconnecting/offline) terlihat jelas di UI
- [ ] Semua test di §7 lulus
- [ ] README lengkap sesuai §9

---

## 12. Execution Plan (Urutan Wajib)

1. **Fondasi** — schema DB, docker-compose, auth (register/login/refresh), health check
2. **Sync dasar** — WS single-instance, SYNC_STEP_1/2, UPDATE relay, ProseMirror + custom Yjs provider
3. **Presence** — awareness protocol, live cursor, avatar list, throttling
4. **Persistence** — snapshot + event log, replay saat load, background flush worker
5. **Version history UI** — timeline, preview, restore
6. **Undo/redo** — per-user scoping, test skenario selective undo
7. **Auth hardening** — role enforcement server-side, workspace invite flow
8. **Task board** — schema, fractional indexing, drag-drop, optimistic concurrency
9. **Connection resilience** — reconnect logic, status indicator UI, heartbeat
10. **Testing** — unit, integration, E2E (§7 lengkap)
11. **UI polish** — sistem desain §5 diterapkan konsisten di semua halaman
12. **Dokumentasi & deployment** — README, `.env.example`, docker-compose final, opsional deploy ke platform seperti Railway/Fly.io

---

*Dokumen ini final dan lengkap untuk memulai development. Update hanya dilakukan lewat diskusi eksplisit, bukan penyesuaian sepihak oleh agent.*