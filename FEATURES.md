# Pulse — Feature Overview

## Architecture

| Layer | Tech | Entry |
|---|---|---|
| Frontend | Next.js 14 (App Router) + React 18 | `web/` — port 3000 |
| Backend | Go 1.22+ (chi router, pgx) | `server/` — port 9000 |
| Database | PostgreSQL 16 | Port 5433 (host) / 5432 (container) |
| Cache | Redis 7 | Port 6380 (host) / 6379 (container) |
| Real-time | Yjs (CRDT) via WebSocket | `/ws/doc/{id}`, `/ws/board/{id}` |
| Auth | JWT (access) + rotating refresh tokens | Cookie-based refresh, in-memory access |

All API calls are proxied through Next.js rewrites (same-origin, no CORS). WebSocket connects directly to `ws://localhost:9000`.

---

## Feature List

### 1. Authentication & User Management

| Feature | Details |
|---|---|
| Register | `POST /auth/register` — email, name, password |
| Login | `POST /auth/login` — returns JWT + refresh token cookie |
| Refresh | `POST /auth/refresh` — rotating refresh token family with reuse detection |
| Logout | `POST /auth/logout` — revokes refresh token family |
| Profile | `PATCH /users/me` — update name |
| Session | `GET /me` — current user info |

**Security**: JWT stored in-memory (not localStorage). Refresh token in `pulse_refresh` cookie (`Path=/`, `HttpOnly`, `Secure` in prod). Middleware redirects unauthenticated requests to `/login`.

---

### 2. Workspaces

| Feature | Details |
|---|---|
| Create | `POST /api/workspaces` — auto-join as owner |
| List | `GET /api/workspaces` — workspaces where user is member |
| Get | `GET /api/workspaces/{id}` — single workspace details |
| Members | `GET /api/workspaces/{id}/members` — list members with roles |
| Role update | `PATCH /api/workspaces/{id}/members/{userId}` — owner-only |
| Remove member | `DELETE /api/workspaces/{id}/members/{userId}` — owner-only |
| Roles | `owner`, `editor`, `viewer` (stored as PG enum) |

First workspace created automatically on user registration.

---

### 3. Documents (Real-time Collaborative Editor)

| Feature | Details |
|---|---|
| Create | `POST /api/workspaces/{id}/documents` |
| List | `GET /api/workspaces/{id}/documents` |
| Rename | `PATCH /api/documents/{id}` — `editor` role minimum |
| Delete | `DELETE /api/documents/{id}` — `editor` role minimum |
| Real-time editing | Yjs CRDT via `/ws/doc/{id}` WebSocket |
| Awareness | Presence indicators (cursor, selection) — throttled at 20 msg/s client, 50ms batch server |
| Undo/Redo | `yUndoPlugin` with `trackedOrigins` per-user |
| Snapshots | Yjs state snapshots for rewind |
| Restore | `POST /api/documents/{id}/snapshots/{snapId}/restore` — `editor` role minimum |
| History | `/w/{wsId}/doc/{docId}/history` — snapshot timeline |

#### Load Flow (Fase 4)
1. Client connects WebSocket
2. Server loads latest `document_snapshot` + replays `document_events` after snapshot
3. State survives server restart

---

### 4. Kanban Board

| Feature | Details |
|---|---|
| Create board | `POST /api/workspaces/{id}/boards` |
| List boards | `GET /api/workspaces/{id}/boards` |
| Get board (with columns + tasks) | `GET /api/boards/{id}` |
| Create column | `POST /api/boards/{id}/columns` — optional `position` |
| Update column | `PATCH /api/columns/{id}` — title and/or position |
| Delete column | `DELETE /api/columns/{id}` — cascades to tasks |
| Create task | `POST /api/columns/{id}/tasks` — optional `position` |
| Update task | `PATCH /api/tasks/{id}` — title, description, column, position + version check |
| Delete task | `DELETE /api/tasks/{id}` |
| Task drag-and-drop | Cross-column + same-column reorder — fractional midpoint `(prev+next)/2` |
| Column drag-and-drop | Reorder columns — fractional midpoint |
| Visual drop indicator | Blue line between tasks during drag |
| Empty state | "Drop here" dashed zone when column empty + drag active |
| Real-time | `/ws/board/{id}` WebSocket — broadcasts create/update/delete events |

**Positioning**: Uses `DOUBLE PRECISION` with fractional indexing (midpoint between neighbors). Normalised to 9 decimal places.

**Concurrency**: Optimistic locking via `version` field (409 Conflict on stale update).

---

### 5. Invite System

| Feature | Details |
|---|---|
| Create invite | `POST /api/workspaces/{id}/invites` — email + role (`editor`/`viewer`) |
| Get invite (public) | `GET /invites/{token}` — workspace name, inviter name, role, status |
| Accept invite | `POST /invites/{token}/accept` — join workspace |
| Invite link | Auto-copied in workspace settings |
| `invited_by` | Tracks who sent the invite (migration 00004) |

**Flow**: Owner/editor creates invite → shares link → recipient opens `/invite/{token}` → accepts → redirected to workspace.

---

### 6. Authorization

| Scope | Viewer | Editor | Owner |
|---|---|---|---|
| View documents/boards | ✓ | ✓ | ✓ |
| Edit documents | ✗ | ✓ | ✓ |
| Delete documents | ✗ | ✓ | ✓ |
| Create/update/delete columns | ✗ | ✓ | ✓ |
| Create/update/delete tasks | ✗ | ✓ | ✓ |
| Manage members | ✗ | ✗ | ✓ |
| Invite users | ✗ | ✓ | ✓ |

All REST mutations check role via `workspaces.GetMemberRole`. Document mutations use `documents.MemberRole`.

---

### 7. WebSocket

| Endpoint | Protocol | Purpose |
|---|---|---|
| `/ws/doc/{documentId}` | Yjs sync protocol | Real-time document editing |
| `/ws/board/{boardId}` | Custom JSON messages | Real-time board updates |

Auth via `?token=` query param (JWT), validated on connection.

---

### 8. Health & Monitoring

| Endpoint | Purpose |
|---|---|
| `GET /health` | DB + Redis ping, returns status + component reports |

---

## Frontend Pages

| Route | Page | Description |
|---|---|---|
| `/` | Landing | Root/home |
| `/login` | Login | Sign in |
| `/register` | Register | Create account |
| `/dashboard` | Dashboard | Workspace list |
| `/settings/profile` | Profile | Edit name |
| `/invite/[token]` | Invite | Accept/decline invite |
| `/w/[workspaceId]` | Workspace | Documents + boards list |
| `/w/[workspaceId]/settings` | Workspace settings | Invite link, name, members |
| `/w/[workspaceId]/doc/[docId]` | Document editor | Yjs collaborative editor |
| `/w/[workspaceId]/doc/[docId]/history` | History | Snapshot timeline |
| `/w/[workspaceId]/board/[boardId]` | Board | Kanban board |

---

## Database Migrations

| # | File | Changes |
|---|---|---|
| 1 | `00001_init.sql` | Core: users, workspaces, documents, snapshots, events, refresh_tokens |
| 2 | `00002_boards.sql` | Board: boards, board_columns, tasks |
| 3 | `00003_invites.sql` | Invite: workspace_invites |
| 4 | `00004_invite_invited_by.sql` | Invite enhancement: `invited_by` column |

---

## Key Design Decisions

- **No ORM** — raw SQL via pgx, full control over queries
- **Fractional indexing** — `DOUBLE PRECISION` positions with midpoint calculation for insertion without re-indexing
- **Optimistic concurrency** — `version` field on tasks, 409 on conflict
- **Snapshot+replay** — document state = latest snapshot + replayed events since snapshot
- **Cookie-based refresh** — refresh token in `HttpOnly` cookie, access token in memory (XSS-resistant)
- **Same-origin API** — all `/api/*` proxied via Next.js rewrites (no CORS configuration needed)
- **Awareness throttling** — client limits to 20 msg/s, server batches 50ms window
