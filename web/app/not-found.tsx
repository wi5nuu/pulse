import Link from 'next/link'

// 404 page — halaman tidak ditemukan (sebelumnya tidak ada).
export default function NotFound() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="text-center px-6">
        <p className="text-5xl font-bold text-gray-300 mb-4">404</p>
        <h2 className="text-xl font-semibold text-gray-900 mb-2">Page not found</h2>
        <p className="text-gray-500 text-sm mb-6">
          The page you are looking for does not exist or has been moved.
        </p>
        <Link
          href="/"
          className="inline-block px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 transition-colors text-sm"
        >
          Go home
        </Link>
      </div>
    </div>
  )
}