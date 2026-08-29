'use client'

import { useEffect } from 'react'

// Error boundary level route (Next.js App Router): menangkap runtime error
// dari halaman — sebelumnya error React menampilkan halaman putih.
export default function Error({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    console.error('Page error:', error)
  }, [error])

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="text-center px-6">
        <h2 className="text-2xl font-semibold text-gray-900 mb-2">Something went wrong</h2>
        <p className="text-gray-500 mb-6 text-sm max-w-md mx-auto">
          An unexpected error occurred while loading this page. Please try again.
        </p>
        <button
          onClick={reset}
          className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 transition-colors text-sm"
        >
          Try again
        </button>
      </div>
    </div>
  )
}