'use client'

import { useEffect, useState, useCallback, useRef, type MutableRefObject } from 'react'
import { useParams } from 'next/navigation'
import { apiGet, apiPost, apiPatch, apiDelete, getAccessToken, refreshAccessToken, notifySessionExpired, WS_BASE } from '@/lib/api-client'
import toast from 'react-hot-toast'

interface Column {
  id: string
  title: string
  position: number
}

interface Task {
  id: string
  title: string
  description: string | null
  assigneeId: string | null
  position: number
  version: number
  columnId: string
}

type ModalType = 'addTask' | 'editTask' | 'deleteTask' | 'deleteColumn' | null

const BASE_RECONNECT_DELAY = 1000
const MAX_RECONNECT_DELAY = 30000

export default function BoardPage() {
  const params = useParams()
  const boardId = params.boardId as string
  const [columns, setColumns] = useState<Column[]>([])
  const [tasks, setTasks] = useState<Task[]>([])
  const [loading, setLoading] = useState(true)
  const [newColTitle, setNewColTitle] = useState('')
  const [draggedTask, setDraggedTask] = useState<string | null>(null)
  const [draggedCol, setDraggedCol] = useState<string | null>(null)
  const [dropCol, setDropCol] = useState<string | null>(null)
  const [dropIdx, setDropIdx] = useState<number | null>(null)
  const [dragOverCol, setDragOverCol] = useState<string | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const wsReconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const wsAttemptsRef = useRef(0)
  const wsPingRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const wsLastMsgRef = useRef(0)
  const disposedRef = useRef(false)
  const taskRefs = useRef(new Map<string, HTMLDivElement>()) as MutableRefObject<Map<string, HTMLDivElement>>
  const colRefs = useRef(new Map<string, HTMLDivElement>()) as MutableRefObject<Map<string, HTMLDivElement>>

  // Modal state
  const [modal, setModal] = useState<ModalType>(null)
  const [modalColumnId, setModalColumnId] = useState<string | null>(null)
  const [modalTaskId, setModalTaskId] = useState<string | null>(null)
  const [modalInput, setModalInput] = useState('')
  const [modalSubmitting, setModalSubmitting] = useState(false)

  const loadBoard = useCallback(async () => {
    try {
      const data = await apiGet<{ columns: Column[]; tasks: Task[] }>(`/api/boards/${boardId}`)
      setColumns([...data.columns].sort((a, b) => a.position - b.position))
      setTasks(data.tasks)
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setLoading(false)
    }
  }, [boardId])

  useEffect(() => {
    loadBoard()
  }, [loadBoard])

  // WebSocket with exponential backoff reconnection
  const connectWS = useCallback(() => {
    if (disposedRef.current) return
    const token = getAccessToken()
    if (!token || !boardId) return

    const wsUrl = `${WS_BASE}/ws/board/${boardId}?token=${token}`
    const ws = new WebSocket(wsUrl)
    wsRef.current = ws

    ws.onmessage = (event) => {
      wsLastMsgRef.current = Date.now()
      try {
        const msg = JSON.parse(event.data)
        if (msg.type === 'task_updated' || msg.type === 'task_created' || msg.type === 'task_deleted' ||
            msg.type === 'column_created' || msg.type === 'column_updated' || msg.type === 'column_deleted') {
          loadBoard()
        }
      } catch {
        // ignore malformed messages
      }
    }

    ws.onopen = () => {
      wsAttemptsRef.current = 0
      wsLastMsgRef.current = Date.now()
      // FIX M3: server memutus koneksi board setelah 90s idle (read deadline)
      // karena tidak ada traffic. Kirim ping 30s + deteksi koneksi zombie:
      // jika tidak ada pesan dari server dalam 75s, tutup paksa → reconnect.
      if (wsPingRef.current) clearInterval(wsPingRef.current)
      wsPingRef.current = setInterval(() => {
        if (wsRef.current?.readyState === WebSocket.OPEN) {
          wsRef.current.send(JSON.stringify({ type: 'ping' }))
          if (Date.now() - wsLastMsgRef.current > 75000) {
            wsRef.current.close()
          }
        }
      }, 30000)
      // FIX: setelah reconnect, muat ulang board — perubahan user lain yang
      // terjadi saat koneksi putus tidak ter-refetch sampai ada broadcast.
      loadBoard()
    }

    ws.onclose = () => {
      wsRef.current = null
      if (wsPingRef.current) {
        clearInterval(wsPingRef.current)
        wsPingRef.current = null
      }
      if (wsReconnectRef.current) {
        clearTimeout(wsReconnectRef.current)
        wsReconnectRef.current = null
      }
      // Setelah unmount jangan reconnect — kalau tidak, tiap kunjungan
      // membocorkan satu WebSocket + timer yang hidup terus.
      if (disposedRef.current) return
      const delay = Math.min(
        BASE_RECONNECT_DELAY * Math.pow(2, wsAttemptsRef.current),
        MAX_RECONNECT_DELAY,
      )
      wsAttemptsRef.current++
      // FIX race: refresh token harus selesai SEBELUM reconnect — bukan
      // fire-and-forget yang bisa membuat reconnect pakai token basi.
      wsReconnectRef.current = setTimeout(async () => {
        if (wsAttemptsRef.current > 0 && wsAttemptsRef.current % 3 === 0) {
          const token = await refreshAccessToken()
          if (!token) {
            // Session mati — berhenti reconnect + trigger logout global.
            disposedRef.current = true
            notifySessionExpired()
            return
          }
        }
        connectWS()
      }, delay)
    }

    return ws
  }, [boardId, loadBoard])

  useEffect(() => {
    disposedRef.current = false
    const ws = connectWS()
    return () => {
      disposedRef.current = true
      if (wsReconnectRef.current) clearTimeout(wsReconnectRef.current)
      wsReconnectRef.current = null
      if (wsPingRef.current) {
        clearInterval(wsPingRef.current)
        wsPingRef.current = null
      }
      if (ws) ws.close()
      wsRef.current = null
      wsAttemptsRef.current = 0
    }
  }, [connectWS])

  const computeDropIdx = (colId: string, clientY: number): number => {
    const colTasks = tasks
      .filter((t) => t.columnId === colId && t.id !== draggedTask)
      .sort((a, b) => a.position - b.position)

    const taskEles = colTasks
      .map((t) => ({ id: t.id, el: taskRefs.current.get(t.id) }))
      .filter((x): x is { id: string; el: HTMLDivElement } => x.el != null)

    for (let i = 0; i < taskEles.length; i++) {
      const rect = taskEles[i].el.getBoundingClientRect()
      if (clientY < rect.top + rect.height / 2) {
        return i
      }
    }
    return colTasks.length
  }

  const handleTaskDragOver = (e: React.DragEvent, colId: string) => {
    e.preventDefault()
    const idx = computeDropIdx(colId, e.clientY)
    setDropCol(colId)
    setDropIdx(idx)
    setDragOverCol(null)
  }

  const handleTaskDrop = async (e: React.DragEvent, columnId: string) => {
    e.preventDefault()
    if (!draggedTask) return
    const task = tasks.find((t) => t.id === draggedTask)
    if (!task) { setDraggedTask(null); return }

    const colTasks = tasks
      .filter((t) => t.columnId === columnId && t.id !== draggedTask)
      .sort((a, b) => a.position - b.position)

    const insertIdx = computeDropIdx(columnId, e.clientY)

    let newPos: number
    if (colTasks.length === 0) {
      newPos = 0
    } else if (insertIdx === 0) {
      newPos = colTasks[0].position / 2
    } else if (insertIdx >= colTasks.length) {
      newPos = colTasks[colTasks.length - 1].position + 1
    } else {
      newPos = (colTasks[insertIdx - 1].position + colTasks[insertIdx].position) / 2
    }

    newPos = Math.round(newPos * 1e9) / 1e9

    try {
      await apiPatch(`/api/tasks/${draggedTask}`, {
        columnId,
        position: newPos,
        version: task.version,
      })
      setTasks((prev) =>
        prev.map((t) =>
          t.id === draggedTask ? { ...t, columnId, position: newPos, version: t.version + 1 } : t,
        ),
      )
    } catch (err: any) {
      toast.error(err.message)
      loadBoard()
    }
    setDraggedTask(null)
    setDropCol(null)
    setDropIdx(null)
  }

  // Column drag-and-drop
  const handleColDragStart = (colId: string) => {
    setDraggedCol(colId)
  }

  const handleColDragOver = (e: React.DragEvent, colId: string) => {
    if (!draggedCol || draggedCol === colId) return
    e.preventDefault()
    setDragOverCol(colId)
  }

  const handleColDrop = async (e: React.DragEvent, colId: string) => {
    e.preventDefault()
    if (!draggedCol || draggedCol === colId) { setDraggedCol(null); setDragOverCol(null); return }

    // Jangan mutate state array in-place (columns.sort memutasi array React state).
    const sorted = [...columns].sort((a, b) => a.position - b.position)
    const fromIdx = sorted.findIndex((c) => c.id === draggedCol)
    const toIdx = sorted.findIndex((c) => c.id === colId)
    if (fromIdx === -1 || toIdx === -1) { setDraggedCol(null); setDragOverCol(null); return }

    let newPos: number
    if (toIdx === 0) {
      newPos = sorted[0].position / 2
    } else if (toIdx >= sorted.length - 1) {
      newPos = sorted[sorted.length - 1].position + 1
    } else {
      const after = toIdx > fromIdx ? toIdx + 1 : toIdx
      newPos = (sorted[after - 1].position + sorted[after].position) / 2
    }
    newPos = Math.round(newPos * 1e9) / 1e9

    try {
      await apiPatch(`/api/columns/${draggedCol}`, { position: newPos })
      setColumns((prev) =>
        prev.map((c) => (c.id === draggedCol ? { ...c, position: newPos } : c)),
      )
    } catch (err: any) {
      toast.error(err.message)
      loadBoard()
    }
    setDraggedCol(null)
    setDragOverCol(null)
  }

  const handleAddColumn = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newColTitle.trim()) return
    try {
      const data = await apiPost<{ column: Column }>(`/api/boards/${boardId}/columns`, {
        title: newColTitle,
      })
      setColumns((prev) => [...prev, data.column])
      setNewColTitle('')
    } catch (err: any) {
      toast.error(err.message)
    }
  }

  const openAddTask = (columnId: string) => {
    setModalColumnId(columnId)
    setModalInput('')
    setModal('addTask')
  }

  const handleAddTask = async () => {
    if (!modalInput.trim() || !modalColumnId) return
    setModalSubmitting(true)
    try {
      const data = await apiPost<{ task: Task }>(`/api/columns/${modalColumnId}/tasks`, { title: modalInput })
      setTasks((prev) => [...prev, data.task])
      setModal(null)
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setModalSubmitting(false)
    }
  }

  const openEditTask = (taskId: string) => {
    const task = tasks.find((t) => t.id === taskId)
    if (!task) return
    setModalTaskId(taskId)
    setModalInput(task.title)
    setModal('editTask')
  }

  const handleEditTask = async () => {
    if (!modalInput.trim() || !modalTaskId) return
    const task = tasks.find((t) => t.id === modalTaskId)
    if (!task) return
    setModalSubmitting(true)
    try {
      await apiPatch(`/api/tasks/${modalTaskId}`, { title: modalInput, version: task.version })
      setTasks((prev) =>
        prev.map((t) => (t.id === modalTaskId ? { ...t, title: modalInput, version: t.version + 1 } : t)),
      )
      setModal(null)
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setModalSubmitting(false)
    }
  }

  const openDeleteTask = (taskId: string) => {
    setModalTaskId(taskId)
    setModal('deleteTask')
  }

  const handleDeleteTask = async () => {
    if (!modalTaskId) return
    setModalSubmitting(true)
    try {
      await apiDelete(`/api/tasks/${modalTaskId}`)
      setTasks((prev) => prev.filter((t) => t.id !== modalTaskId))
      setModal(null)
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setModalSubmitting(false)
    }
  }

  const openDeleteColumn = (columnId: string) => {
    setModalColumnId(columnId)
    setModal('deleteColumn')
  }

  const handleDeleteColumn = async () => {
    if (!modalColumnId) return
    setModalSubmitting(true)
    try {
      await apiDelete(`/api/columns/${modalColumnId}`)
      setColumns((prev) => prev.filter((c) => c.id !== modalColumnId))
      setTasks((prev) => prev.filter((t) => t.columnId !== modalColumnId))
      setModal(null)
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setModalSubmitting(false)
    }
  }

  const tasksByColumn = (colId: string) =>
    tasks.filter((t) => t.columnId === colId).sort((a, b) => a.position - b.position)

  if (loading) {
    return (
      <div className="flex-1 overflow-x-auto p-6">
        <div className="flex gap-4 h-full">
          {[1, 2, 3].map((i) => (
            <div key={i} className="skeleton w-72 h-full" />
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-x-auto p-6">
      <div className="flex gap-4 h-full items-start">
        {columns.map((col) => (
          <div
            key={col.id}
            ref={(el) => { if (el) colRefs.current.set(col.id, el); else colRefs.current.delete(col.id) }}
            data-col-id={col.id}
            className={`bg-gray-100 rounded w-72 flex-shrink-0 flex flex-col max-h-full transition-shadow ${
              dragOverCol === col.id ? 'ring-2 ring-blue-400' : ''
            }`}
            onDragOver={(e) => handleColDragOver(e, col.id)}
            onDrop={(e) => draggedCol ? handleColDrop(e, col.id) : handleTaskDrop(e, col.id)}
          >
            <div
              className="px-3 py-2 font-medium text-sm text-gray-700 border-b flex items-center justify-between cursor-grab active:cursor-grabbing"
              draggable
              onDragStart={() => handleColDragStart(col.id)}
            >
              <span>{col.title}</span>
              <div className="flex items-center gap-2">
                <span className="text-xs text-gray-400">{tasksByColumn(col.id).length}</span>
                <button className="text-xs text-red-400 hover:text-red-600" onClick={() => openDeleteColumn(col.id)}>
                  ✕
                </button>
              </div>
            </div>
            <div
              className="flex-1 overflow-y-auto p-2 space-y-2"
              onDragOver={(e) => handleTaskDragOver(e, col.id)}
            >
              {tasksByColumn(col.id).length === 0 && draggedTask && dropCol === col.id && (
                <div className="border-2 border-dashed border-blue-300 rounded p-4 text-xs text-blue-400 text-center">
                  Drop here
                </div>
              )}
              {tasksByColumn(col.id).length === 0 && !(draggedTask && dropCol === col.id) && (
                <div className="text-xs text-gray-300 text-center py-4">No tasks yet</div>
              )}
              {tasksByColumn(col.id).map((task, idx) => (
                <div key={task.id}>
                  {draggedTask && dropCol === col.id && dropIdx === idx && (
                    <div className="h-0.5 bg-blue-500 rounded" />
                  )}
                  <div
                    ref={(el) => { if (el) taskRefs.current.set(task.id, el); else taskRefs.current.delete(task.id) }}
                    className="card p-2 cursor-grab active:cursor-grabbing text-sm"
                    draggable
                    onDragStart={() => setDraggedTask(task.id)}
                  >
                    <div className="font-medium" onDoubleClick={() => openEditTask(task.id)}>{task.title}</div>
                    {task.description && (
                      <div className="text-xs text-gray-500 mt-1">{task.description}</div>
                    )}
                    <button
                      className="text-xs text-red-500 mt-1 hover:underline"
                      onClick={() => openDeleteTask(task.id)}
                    >
                      Delete
                    </button>
                  </div>
                </div>
              ))}
              {draggedTask && dropCol === col.id && dropIdx === tasksByColumn(col.id).length && (
                <div className="h-0.5 bg-blue-500 rounded" />
              )}
              <button
                className="w-full text-xs text-gray-400 hover:text-gray-600 py-1"
                onClick={() => openAddTask(col.id)}
              >
                + Add task
              </button>
            </div>
          </div>
        ))}

        <form onSubmit={handleAddColumn} className="flex-shrink-0 w-72">
          <input
            className="input text-sm"
            placeholder="+ Add column"
            value={newColTitle}
            onChange={(e) => setNewColTitle(e.target.value)}
          />
        </form>
      </div>

      {/* Modal overlay */}
      {modal && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
          onClick={() => setModal(null)}
          role="presentation"
        >
          <div
            className="bg-white rounded shadow-lg w-full max-w-sm p-6"
            role="dialog"
            aria-modal="true"
            aria-label={modal === 'addTask' ? 'Add task' : modal === 'editTask' ? 'Edit task' : 'Confirm deletion'}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              // FIX aksesibilitas: Escape menutup modal (sebelumnya tidak ada).
              if (e.key === 'Escape') setModal(null)
            }}
          >
            {modal === 'addTask' && (
              <>
                <h3 className="text-base font-semibold mb-3">Add Task</h3>
                <input
                  className="input mb-3"
                  placeholder="Task title"
                  value={modalInput}
                  onChange={(e) => setModalInput(e.target.value)}
                  autoFocus
                  onKeyDown={(e) => { if (e.key === 'Enter') handleAddTask() }}
                />
                <div className="flex justify-end gap-2">
                  <button className="btn-secondary" onClick={() => setModal(null)}>Cancel</button>
                  <button className="btn-primary" onClick={handleAddTask} disabled={modalSubmitting || !modalInput.trim()}>
                    {modalSubmitting ? 'Adding...' : 'Add'}
                  </button>
                </div>
              </>
            )}
            {modal === 'editTask' && (
              <>
                <h3 className="text-base font-semibold mb-3">Edit Task</h3>
                <input
                  className="input mb-3"
                  placeholder="Task title"
                  value={modalInput}
                  onChange={(e) => setModalInput(e.target.value)}
                  autoFocus
                  onKeyDown={(e) => { if (e.key === 'Enter') handleEditTask() }}
                />
                <div className="flex justify-end gap-2">
                  <button className="btn-secondary" onClick={() => setModal(null)}>Cancel</button>
                  <button className="btn-primary" onClick={handleEditTask} disabled={modalSubmitting || !modalInput.trim()}>
                    {modalSubmitting ? 'Saving...' : 'Save'}
                  </button>
                </div>
              </>
            )}
            {modal === 'deleteTask' && (
              <>
                <h3 className="text-base font-semibold mb-3">Delete Task</h3>
                <p className="text-sm text-gray-600 mb-4">Are you sure you want to delete this task?</p>
                <div className="flex justify-end gap-2">
                  <button className="btn-secondary" onClick={() => setModal(null)}>Cancel</button>
                  <button className="btn-danger" onClick={handleDeleteTask} disabled={modalSubmitting}>
                    {modalSubmitting ? 'Deleting...' : 'Delete'}
                  </button>
                </div>
              </>
            )}
            {modal === 'deleteColumn' && (
              <>
                <h3 className="text-base font-semibold mb-3">Delete Column</h3>
                <p className="text-sm text-gray-600 mb-4">Are you sure you want to delete this column and all its tasks?</p>
                <div className="flex justify-end gap-2">
                  <button className="btn-secondary" onClick={() => setModal(null)}>Cancel</button>
                  <button className="btn-danger" onClick={handleDeleteColumn} disabled={modalSubmitting}>
                    {modalSubmitting ? 'Deleting...' : 'Delete'}
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
