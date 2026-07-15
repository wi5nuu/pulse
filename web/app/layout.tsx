import type { Metadata } from 'next'
import '@/styles/globals.css'
import 'prosemirror-view/style/prosemirror.css'
import 'prosemirror-menu/style/menu.css'
import { Toaster } from 'react-hot-toast'

export const metadata: Metadata = {
  title: 'Pulse',
  description: 'Real-time collaborative workspace',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-screen bg-white text-gray-900 antialiased">
        {children}
        <Toaster position="bottom-right" toastOptions={{ duration: 4000 }} />
      </body>
    </html>
  )
}
