'use client'

import { useState } from 'react'
import { AuthGuard } from '@/components/auth-guard'
import { useAuthStore } from '@/store/auth'
import { apiPatch } from '@/lib/api-client'
import toast from 'react-hot-toast'

export default function ProfilePage() {
  const user = useAuthStore((s) => s.user)
  const [name, setName] = useState(user?.name || '')
  const [saving, setSaving] = useState(false)

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setSaving(true)
    try {
      await apiPatch('/api/users/me', { name })
      toast.success('Profile updated')
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <AuthGuard>
      <div className="min-h-screen bg-gray-50 px-6 py-8">
        <div className="max-w-md mx-auto">
          <h1 className="text-xl font-semibold mb-6">Profile Settings</h1>
          <form onSubmit={handleSave} className="card space-y-4">
            <div>
              <label className="label">Email</label>
              <input className="input" value={user?.email || ''} disabled />
            </div>
            <div>
              <label className="label">Display Name</label>
              <input
                className="input"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
            <button className="btn-primary" disabled={saving} type="submit">
              {saving ? 'Saving...' : 'Save'}
            </button>
          </form>
        </div>
      </div>
    </AuthGuard>
  )
}
