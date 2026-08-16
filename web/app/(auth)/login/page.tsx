'use client'

import { useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import Link from 'next/link'
import toast from 'react-hot-toast'
import { login } from '@/lib/api-client'
import { useAuthStore } from '@/store/auth'

export default function LoginPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const setUser = useAuthStore((s) => s.setUser)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      const data = await login(email, password)
      setUser(data.user, data.accessToken)
      toast.success('Logged in')
      // Halaman invite mengarahkan ke /login?redirect=/invite/{token} —
      // jangan buang redirect itu, kembalikan user ke tujuan semula.
      const redirect = searchParams.get('redirect')
      router.push(redirect || '/dashboard')
    } catch (err: any) {
      toast.error(err.message)
    } finally {
      setSubmitting(false)
    }
  }

  const fillDemoCredentials = () => {
    setEmail('demo@pulse.test')
    setPassword('demo1234')
    toast.success('Demo credentials filled - click Sign in')
  }

  return (
    <div>
      <h1 className="text-2xl font-semibold text-center mb-6">Sign in to Pulse</h1>
      <form onSubmit={handleSubmit} className="card space-y-4">
        <div>
          <label className="label">Email</label>
          <input
            className="input"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoFocus
          />
        </div>
        <div>
          <label className="label">Password</label>
          <input
            className="input"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </div>
        <button className="btn-primary w-full" disabled={submitting} type="submit">
          {submitting ? 'Signing in...' : 'Sign in'}
        </button>
        
        <div className="relative">
          <div className="absolute inset-0 flex items-center">
            <div className="w-full border-t border-gray-300"></div>
          </div>
          <div className="relative flex justify-center text-sm">
            <span className="px-2 bg-white text-gray-500">For Testing</span>
          </div>
        </div>

        <button
          type="button"
          onClick={fillDemoCredentials}
          className="w-full px-4 py-2 border border-gray-300 rounded-md shadow-sm text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-accent-500"
          disabled={submitting}
        >
          Fill Demo Credentials
        </button>

        <div className="text-xs text-gray-400 text-center space-y-1">
          <p>Demo account (create if not exists):</p>
          <p className="font-mono">demo@pulse.test / demo1234</p>
        </div>

        <p className="text-sm text-gray-500 text-center">
          Don&apos;t have an account?{' '}
          <Link href="/register" className="text-accent-600 hover:underline">
            Register
          </Link>
        </p>
      </form>
    </div>
  )
}
