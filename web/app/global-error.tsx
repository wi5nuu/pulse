'use client'

// Global error boundary (root layout level) — fallback terakhir jika error.tsx
// tingkat route gagal. Menampilkan halaman pemulihan sederhana.
export default function GlobalError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  void error
  return (
    <html lang="en">
      <body style={{ fontFamily: 'system-ui, sans-serif', margin: 0 }}>
        <div
          style={{
            minHeight: '100vh',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            background: '#f9fafb',
          }}
        >
          <div style={{ textAlign: 'center', padding: '0 24px' }}>
            <h2 style={{ fontSize: 24, fontWeight: 600, color: '#111827', marginBottom: 8 }}>
              Something went wrong
            </h2>
            <p style={{ color: '#6b7280', fontSize: 14, marginBottom: 24 }}>
              A critical error occurred. Please reload the page.
            </p>
            <button
              onClick={reset}
              style={{
                padding: '8px 16px',
                background: '#2563eb',
                color: '#fff',
                border: 'none',
                borderRadius: 6,
                fontSize: 14,
                cursor: 'pointer',
              }}
            >
              Reload
            </button>
          </div>
        </div>
      </body>
    </html>
  )
}