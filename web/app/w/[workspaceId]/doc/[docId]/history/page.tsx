'use client'

import { useEffect, useState } from 'react'
import { useParams } from 'next/navigation'
import { apiGet, apiPost } from '@/lib/api-client'
import toast from 'react-hot-toast'

interface Snapshot {
  id: number
  createdAt: string
  eventCount: number
}

export default function HistoryPage() {
  const params = useParams()
  const docId = params.docId as string
  const [snapshots, setSnapshots] = useState<Snapshot[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [restoringId, setRestoringId] = useState<number | null>(null)
  const [confirmId, setConfirmId] = useState<number | null>(null)

  useEffect(() => {
    apiGet<{ snapshots: Snapshot[] }>(`/api/documents/${docId}/snapshots`)
      .then((data) => setSnapshots(data.snapshots))
      .catch((err: any) => {
        // FIX: gagal fetch tidak boleh tampil sebagai empty state.
        setLoadError(err.message || 'Failed to load history')
      })
      .finally(() => setLoading(false))
  }, [docId])

  const handleRestore = async (snapshotId: number) => {
    setRestoringId(snapshotId)
    setConfirmId(null)
    try {
      await apiPost(`/api/documents/${docId}/snapshots/${snapshotId}/restore`)
      toast.success('Document restored')
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setRestoringId(null)
    }
  }

  return (
    <div className="flex-1 overflow-y-auto px-8 py-6">
      <h2 className="text-lg font-semibold mb-4">Version History</h2>
      {loading ? (
        <div className="space-y-2">
          {[1, 2, 3].map((i) => (
            <div key={i} className="skeleton h-12 w-full" />
          ))}
        </div>
      ) : loadError ? (
        <div className="text-center py-12">
          <p className="text-red-600 mb-2">{loadError}</p>
          <button className="btn-secondary" onClick={() => window.location.reload()}>
            Retry
          </button>
        </div>
      ) : snapshots.length === 0 ? (
        <div className="text-center py-12 text-gray-400">
          <p>No history available yet</p>
          <p className="text-sm">Snapshots are created periodically as you edit</p>
        </div>
      ) : (
        <div className="space-y-2">
          {snapshots.map((s) => (
            <div key={s.id} className="card flex items-center justify-between">
              <div>
                <div className="text-sm font-medium">
                  {new Date(s.createdAt).toLocaleString()}
                </div>
                <div className="text-xs text-gray-400">{s.eventCount} events</div>
              </div>
              <button
                className="btn-secondary text-xs"
                onClick={() => setConfirmId(s.id)}
              >
                Restore
              </button>
            </div>
          ))}
        </div>
      )}

      {/* Restore confirmation modal */}
      {confirmId !== null && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
          onClick={() => setConfirmId(null)}
        >
          <div
            className="bg-white rounded shadow-lg w-full max-w-sm p-6"
            onClick={(e) => e.stopPropagation()}
          >
            <h3 className="text-base font-semibold mb-3">Restore Version</h3>
            <p className="text-sm text-gray-600 mb-4">
              This will replace the current document content with the selected version. All changes after this snapshot will be lost. Continue?
            </p>
            <div className="flex justify-end gap-2">
              <button className="btn-secondary" onClick={() => setConfirmId(null)}>Cancel</button>
              <button
                className="btn-primary"
                onClick={() => handleRestore(confirmId)}
                disabled={restoringId !== null}
              >
                {restoringId === confirmId ? 'Restoring...' : 'Restore'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
