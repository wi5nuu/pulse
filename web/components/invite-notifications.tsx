'use client'

import { useCallback, useEffect, useState } from 'react'
import { apiGet } from '@/lib/api-client'
import Link from 'next/link'

interface PendingInvite {
  id: string
  workspaceId: string
  workspaceName: string
  role: string
  token: string
  invitedByName: string | null
  expiresAt: string
  createdAt: string
}

export function InviteNotifications() {
  const [invites, setInvites] = useState<PendingInvite[]>([])
  const [loading, setLoading] = useState(true)
  const [showDropdown, setShowDropdown] = useState(false)

  const fetchInvites = useCallback(async () => {
    try {
      const data = await apiGet<{ invites: PendingInvite[] }>('/invites/pending')
      setInvites(data.invites || [])
    } catch {
      // 401 → global handler di api-client akan logout & redirect; polling
      // berhenti otomatis karena komponen unmount.
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchInvites()
    // Poll setiap 30 detik untuk update realtime
    const interval = setInterval(fetchInvites, 30000)
    return () => clearInterval(interval)
  }, [fetchInvites])

  // Close dropdown when clicking outside
  useEffect(() => {
    if (!showDropdown) return
    const handleClickOutside = (e: MouseEvent) => {
      const target = e.target as HTMLElement
      if (!target.closest('[data-invite-dropdown]')) {
        setShowDropdown(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [showDropdown])

  const count = invites.length

  return (
    <div className="relative" data-invite-dropdown>
      <button
        onClick={() => setShowDropdown(!showDropdown)}
        className="relative p-2 text-gray-600 hover:text-gray-900 focus:outline-none"
        aria-label="Notifications"
      >
        <svg
          className="w-6 h-6"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"
          />
        </svg>
        {count > 0 && (
          <span className="absolute top-0 right-0 inline-flex items-center justify-center px-2 py-1 text-xs font-bold leading-none text-white transform translate-x-1/2 -translate-y-1/2 bg-red-600 rounded-full">
            {count}
          </span>
        )}
      </button>

      {showDropdown && (
        <div className="absolute right-0 mt-2 w-80 bg-white rounded-lg shadow-lg border border-gray-200 z-50">
          <div className="p-4 border-b border-gray-200">
            <h3 className="text-sm font-semibold text-gray-900">
              Workspace Invitations ({count})
            </h3>
          </div>

          <div className="max-h-96 overflow-y-auto">
            {loading ? (
              <div className="p-4 text-center text-sm text-gray-500">
                Loading...
              </div>
            ) : count === 0 ? (
              <div className="p-4 text-center text-sm text-gray-500">
                No pending invitations
              </div>
            ) : (
              <div className="divide-y divide-gray-100">
                {invites.map((invite) => (
                  <Link
                    key={invite.id}
                    href={`/invite/${invite.token}`}
                    className="block p-4 hover:bg-gray-50 transition-colors"
                    onClick={() => setShowDropdown(false)}
                  >
                    <div className="flex items-start">
                      <div className="flex-1">
                        <p className="text-sm font-medium text-gray-900">
                          {invite.workspaceName}
                        </p>
                        <p className="text-xs text-gray-500 mt-1">
                          {invite.invitedByName
                            ? `Invited by ${invite.invitedByName}`
                            : 'You have been invited'}{' '}
                          as <span className="font-medium">{invite.role}</span>
                        </p>
                        <p className="text-xs text-gray-400 mt-1">
                          {new Date(invite.createdAt).toLocaleDateString()}
                        </p>
                      </div>
                      <svg
                        className="w-5 h-5 text-gray-400"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M9 5l7 7-7 7"
                        />
                      </svg>
                    </div>
                  </Link>
                ))}
              </div>
            )}
          </div>

          {count > 0 && (
            <div className="p-3 border-t border-gray-200 text-center">
              <Link
                href="/invites"
                className="text-sm text-accent-600 hover:text-accent-700 font-medium"
                onClick={() => setShowDropdown(false)}
              >
                View all invitations
              </Link>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
