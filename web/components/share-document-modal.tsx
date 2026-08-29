'use client'

import { useState, useEffect, useCallback } from 'react'
import { apiGet, apiPost, apiDelete } from '@/lib/api-client'
import toast from 'react-hot-toast'

interface ShareDocumentModalProps {
  documentId: string
  isOpen: boolean
  onClose: () => void
}

interface Share {
  id: string
  sharedWithId: string
  sharedWithName: string
  sharedWithEmail: string
  permission: 'view' | 'edit'
  createdAt: string
}

interface LinkShare {
  id: string
  documentId: string
  token: string
  permission: 'view' | 'edit'
  expiresAt: string | null
  createdAt: string
}

export function ShareDocumentModal({ documentId, isOpen, onClose }: ShareDocumentModalProps) {
  const [email, setEmail] = useState('')
  const [permission, setPermission] = useState<'view' | 'edit'>('edit')
  const [shares, setShares] = useState<Share[]>([])
  const [canManage, setCanManage] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [linkShares, setLinkShares] = useState<LinkShare[]>([])
  const [linkPermission, setLinkPermission] = useState<'view' | 'edit'>('edit')
  const [copied, setCopied] = useState(false)

  const loadShares = useCallback(async () => {
    try {
      const res = await apiGet<{ shares: Share[]; canManage: boolean }>(`/api/documents/${documentId}/shares`)
      setShares(res.shares || [])
      setCanManage(res.canManage || false)
    } catch (err: any) {
      console.error('Failed to load shares:', err)
    }
  }, [documentId])

  const loadLinkShares = useCallback(async () => {
    try {
      const res = await apiGet<{ shares: LinkShare[] }>(`/api/documents/${documentId}/linkshare`)
      setLinkShares(res.shares || [])
    } catch {
      /* non-member: abaikan */
    }
  }, [documentId])

  useEffect(() => {
    if (isOpen) {
      loadShares()
      loadLinkShares()
    }
  }, [isOpen, loadShares, loadLinkShares])

  const handleCreateLinkShare = async () => {
    try {
      const res = await apiPost<{ share: LinkShare }>(`/api/documents/${documentId}/linkshare`, {
        permission: linkPermission,
      })
      setLinkShares((prev) => [res.share, ...prev.filter((s) => s.id !== res.share.id)])
      toast.success('Link share dibuat')
    } catch (err: any) {
      toast.error(err.message ?? 'Gagal membuat link share')
    }
  }

  const copyLink = async (token: string) => {
    const url = `${window.location.origin}/l/${token}`
    try {
      await navigator.clipboard.writeText(url)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // Fallback: select text for manual copy
      const input = document.createElement('input')
      input.value = url
      document.body.appendChild(input)
      input.select()
      document.execCommand('copy')
      document.body.removeChild(input)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    }
  }

  const handleDeleteLinkShare = async (shareId: string) => {
    try {
      await apiDelete(`/api/documents/${documentId}/linkshare/${shareId}`)
      setLinkShares((prev) => prev.filter((s) => s.id !== shareId))
    } catch (err: any) {
      toast.error(err.message ?? 'Gagal menghapus link share')
    }
  }

  const handleShare = async () => {
    if (!email.trim()) {
      setError('Please enter an email address')
      return
    }

    setLoading(true)
    setError('')

    try {
      const userRes = await apiGet<{ id: string; name: string; email: string }>(`/api/users/by-email/${encodeURIComponent(email)}`)

      await apiPost(`/api/documents/${documentId}/shares`, {
        userId: userRes.id,
        permission,
      })

      setEmail('')
      setPermission('edit')
      await loadShares()
    } catch (err: any) {
      setError(err.message || 'Failed to share document')
    } finally {
      setLoading(false)
    }
  }

  const handleUnshare = async (userId: string) => {
    try {
      await apiDelete(`/api/documents/${documentId}/shares/${userId}`)
      await loadShares()
    } catch (err: any) {
      setError(err.message || 'Failed to remove access')
    }
  }

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-xl font-semibold">Share Document</h2>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
          >
            <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {canManage && (
          <div className="mb-6">
            <label className="block text-sm font-medium text-gray-700 mb-2">
              Share with
            </label>
            <div className="flex gap-2">
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="Enter email address"
                className="flex-1 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                onKeyDown={(e) => e.key === 'Enter' && handleShare()}
              />
              <select
                value={permission}
                onChange={(e) => setPermission(e.target.value as 'view' | 'edit')}
                className="px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                <option value="edit">Can edit</option>
                <option value="view">Can view</option>
              </select>
            </div>
            <button
              onClick={handleShare}
              disabled={loading}
              className="mt-2 w-full px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading ? 'Sharing...' : 'Share'}
            </button>
            {error && (
              <p className="mt-2 text-sm text-red-600">{error}</p>
            )}
          </div>
        )}

        {canManage && (
          <div className="mb-6 border-t pt-4">
            <h3 className="text-sm font-medium text-gray-700 mb-2">
              Anyone with the link
            </h3>
            <div className="flex gap-2">
              <select
                value={linkPermission}
                onChange={(e) => setLinkPermission(e.target.value as 'view' | 'edit')}
                className="px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
              >
                <option value="edit">Can edit</option>
                <option value="view">Can view</option>
              </select>
              <button
                onClick={handleCreateLinkShare}
                className="flex-1 px-4 py-2 bg-gray-800 text-white text-sm rounded-md hover:bg-gray-900"
              >
                {linkShares.length > 0 ? 'New link share' : 'Create link share'}
              </button>
            </div>
            {copied && <p className="mt-1 text-xs text-green-600">Link copied!</p>}
            <div className="mt-2 space-y-2">
              {linkShares.map((ls) => (
                <div key={ls.id} className="flex items-center justify-between p-2 bg-gray-50 rounded-md">
                  <div className="flex-1 min-w-0">
                    <span className="text-xs font-medium text-gray-700">
                      {ls.permission === 'edit' ? 'Can edit' : 'Can view'}
                    </span>
                    <span className="text-[11px] text-gray-400 ml-2">
                      {ls.expiresAt ? `exp ${new Date(ls.expiresAt).toLocaleDateString()}` : 'no expiry'}
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => copyLink(ls.token)}
                      className="text-xs px-2 py-1 border rounded hover:bg-white"
                    >
                      Copy link
                    </button>
                    <button
                      onClick={() => handleDeleteLinkShare(ls.id)}
                      className="text-red-600 hover:text-red-800 text-xs"
                    >
                      Remove
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        <div>
          <h3 className="text-sm font-medium text-gray-700 mb-3">
            People with access ({shares.length})
          </h3>
          <div className="space-y-2 max-h-64 overflow-y-auto">
            {shares.length === 0 ? (
              <p className="text-sm text-gray-500 text-center py-4">
                No one else has access to this document
              </p>
            ) : (
              shares.map((share) => (
                <div
                  key={share.id}
                  className="flex items-center justify-between p-3 bg-gray-50 rounded-md"
                >
                  <div className="flex-1">
                    <p className="text-sm font-medium text-gray-900">
                      {share.sharedWithName}
                    </p>
                    <p className="text-xs text-gray-500">{share.sharedWithEmail}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-gray-600 px-2 py-1 bg-white rounded border">
                      {share.permission === 'edit' ? 'Can edit' : 'Can view'}
                    </span>
                    {canManage && (
                      <button
                        onClick={() => handleUnshare(share.sharedWithId)}
                        className="text-red-600 hover:text-red-800 text-xs"
                      >
                        Remove
                      </button>
                    )}
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
