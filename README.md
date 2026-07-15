# Pulse

<p>
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go" alt="Go">
  <img src="https://img.shields.io/badge/Next.js-14-000000?logo=next.js" alt="Next.js">
  <img src="https://img.shields.io/badge/PostgreSQL-16-336791?logo=postgresql" alt="PostgreSQL">
  <img src="https://img.shields.io/badge/Redis-7-DC382D?logo=redis" alt="Redis">
  <img src="https://img.shields.io/badge/TypeScript-3178C6?logo=typescript" alt="TypeScript">
  <img src="https://img.shields.io/badge/Tailwind_CSS-06B6D4?logo=tailwindcss" alt="Tailwind CSS">
</p>

> Real-time collaborative workspace — rich-text editor, Kanban board, live presence, and version history. Built with CRDT conflict-free replication (Yjs + ProseMirror).

---

## Features

- **Collaborative Document Editor** — Real-time rich-text editing powered by Yjs CRDT and ProseMirror. Multiple users edit simultaneously with automatic conflict resolution.
- **Live Presence** — See who's online, where their cursor is, and what they're selecting — all in real time with smooth animation.
- **Multiplayer Undo/Redo** — Per-user undo scoping: undo only removes your own changes, not others'.
- **Kanban Board** — Drag-and-drop task management with fractional indexing and optimistic concurrency control.
- **Version History** — Snapshot-based document history with restore capability.
- **Invite System** — Share invite links to collaborate with team members (editor/viewer roles).
- **Role-Based Access Control** — Owner, editor, and viewer roles enforced at the API and WebSocket level.
- **Secure Auth** — JWT access tokens (in-memory) + rotating refresh tokens (httpOnly cookie) with reuse detection.

---

## Screenshots

<!-- TODO: Add screenshots
![Editor](docs/screenshots/editor.png)
![Board](docs/screenshots/board.png)
![Dashboard](docs/screenshots/dashboard.png)
-->

---

## Architecture

```
┌─────────────┐     ┌──────────────────────────────────────┐     ┌──────────┐
│  Next.js    │◄───►│  Go Server                           │◄───►│ Postgres │
│  (Browser)  │ WS  │  ┌─────┐ ┌──────┐ ┌───────────────┐ │     └──────────┘
│  Yjs CRDT   │     │  │chi │ │yws  │ │ persistence   │ │     ┌──────────┐
│  ProseMirror│     │  │REST│ │relay│ │ worker        │ │◄───►│ Redis    │
└─────────────┘     │  └─────┘ └──────┘ └───────────────┘ │     └──────────┘
                    └──────────────────────────────────────┘
```

**Key design decisions:**
- **Server as relay** — no Yjs library on the server; simply forwards binary sync bytes
- **CRDT** — conflict-free merge, no locking, no OT complexity
- **Fractional indexing** — float64 midpoint positions for board tasks; no reindexing needed
- **Refresh token rotation** — reuse detection protects against token theft
- **Selective undo/redo** — `Y.UndoManager` scoped per user origin

---

## Tech Stack

| Layer | Technology |
|---|---|
| Backend API & Real-time | Go 1.25+ (`chi`, `gorilla/websocket`, `pgx`) |
| CRDT | Yjs (client-side) — server as relay |
| Database | PostgreSQL 16 |
| Cache & Pub/Sub | Redis 7 |
| Frontend | Next.js 14 (App Router) + TypeScript |
| Editor | ProseMirror + `y-prosemirror` |
| State Management | Zustand |
| Auth | JWT (HS256, 15m) + rotating refresh tokens |
| Styling | Tailwind CSS |

---

## Project Structure

```
pulse/
├── docker-compose.yml         # Postgres + Redis + server
├── server/
│   ├── cmd/pulse/             # Entrypoint
│   │   ├── main.go
│   │   └── config.go
│   ├── internal/
│   │   ├── auth/              # JWT, password hashing, refresh token service
│   │   ├── boards/            # Board, column, task repository
│   │   ├── config/            # Env-based configuration
│   │   ├── db/                # Connection pool + migration runner
│   │   ├── documents/         # Document + snapshot repository
│   │   ├── health/            # Health check endpoint
│   │   ├── httpapi/           # REST handlers, router, middleware
│   │   ├── migrations/        # SQL migrations (embedded with goose)
│   │   ├── models/            # Domain structs
│   │   ├── persistence/       # Background flush worker
│   │   ├── users/             # User repository
│   │   ├── workspaces/        # Workspace + members repository
│   │   ├── ycodec/            # Yjs wire protocol encoder/decoder
│   │   └── yws/               # WebSocket sync relay + awareness
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
├── web/
│   ├── app/                   # Next.js App Router pages
│   ├── components/            # Shared UI components
│   ├── lib/                   # API client, Yjs provider, utilities
│   ├── store/                 # Zustand stores
│   ├── styles/                # Global CSS
│   └── package.json
├── docs/
│   └── task.md
├── .env.example
├── .gitignore
├── docker-compose.yml
├── FEATURES.md
└── README.md
```

---

## Quick Start

### Prerequisites

- Go 1.22+, Node.js 20+, Docker + Docker Compose

### 1. Infrastructure

```bash
docker compose up -d
```

Starts PostgreSQL (port 5433) and Redis (port 6379).

### 2. Backend

```bash
cd server
cp .env.example .env
go run ./cmd/pulse
```

Server starts on `http://localhost:8080` with automatic database migration.

### 3. Frontend

```bash
cd web
npm install
npm run dev
```

Open `http://localhost:3000`.

---

## API Reference

### Auth

| Method | Path | Description |
|--------|------|-------------|
| POST | `/auth/register` | Register (email, name, password) |
| POST | `/auth/login` | Login — returns accessToken + httpOnly cookie |
| POST | `/auth/refresh` | Rotate refresh token |
| POST | `/auth/logout` | Revoke token family |

### Workspaces

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/workspaces` | List user's workspaces |
| POST | `/api/workspaces` | Create workspace |
| GET | `/api/workspaces/:id` | Get workspace details |
| GET | `/api/workspaces/:id/documents` | List documents |
| POST | `/api/workspaces/:id/documents` | Create document |
| GET | `/api/workspaces/:id/boards` | List boards |
| POST | `/api/workspaces/:id/boards` | Create board |
| GET | `/api/workspaces/:id/members` | List members |
| POST | `/api/workspaces/:id/invites` | Create invite |
| PATCH | `/api/workspaces/:id/members/:userId` | Change role |
| DELETE | `/api/workspaces/:id/members/:userId` | Remove member |

### Documents

| Method | Path | Description |
|--------|------|-------------|
| PATCH | `/api/documents/:id` | Rename |
| DELETE | `/api/documents/:id` | Delete |
| GET | `/api/documents/:id/snapshots` | Version history |
| POST | `/api/documents/:id/snapshots/:snapId/restore` | Restore snapshot |

### Boards

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/boards/:id` | Get board with columns & tasks |
| POST | `/api/boards/:id/columns` | Add column |

### Columns

| Method | Path | Description |
|--------|------|-------------|
| PATCH | `/api/columns/:id` | Rename |
| DELETE | `/api/columns/:id` | Delete (cascade tasks) |
| POST | `/api/columns/:id/tasks` | Add task |

### Tasks

| Method | Path | Description |
|--------|------|-------------|
| PATCH | `/api/tasks/:id` | Update (title, position, column) + version check |
| DELETE | `/api/tasks/:id` | Delete |

### WebSocket

| Endpoint | Protocol | Purpose |
|----------|----------|---------|
| `/ws/doc/:id` | Yjs sync protocol | Real-time document editing |
| `/ws/board/:id` | Custom JSON | Real-time board updates |

Auth via `?token=` query parameter (JWT access token).

---

## Design Decisions

### Why CRDT instead of OT?

CRDT (Yjs) provides conflict-free merge without complex server-side transformation logic. Each client has full editing agency — the server only relays bytes. OT requires server-side transformation that is notoriously difficult to implement correctly.

### Why Fractional Indexing for Boards?

Task board positions use float64 as the index. Inserting between two tasks requires only averaging the neighboring positions — no full-column reindexing needed. Float64 precision is sufficient for ~1000+ tasks per column.

### Why Server as Relay?

The Go server never imports or interprets Yjs state. It receives binary sync messages from clients and broadcasts them to other peers in the same document room. This keeps the server simple and language-agnostic for the real-time layer.

### Selective Undo/Redo

Every Yjs transaction has an `origin` field containing the user ID. `Y.UndoManager` is scoped per origin, so undo only affects the current user's changes. Redo stack is client-side and lost on refresh (deliberate trade-off to simplify persistence).

---

## Known Limitations

- Redo stack is client-side — lost on page refresh
- Presence awareness is ephemeral (not persisted)
- Mobile responsiveness is best-effort (primary target: desktop)
- Single-instance WebSocket (no horizontal scaling yet)
- Dark mode not yet implemented

---

## License

MIT
