'use client'

import { useParams } from 'next/navigation'

export default function WorkspaceHomePage() {
  const params = useParams()

  return (
    <div className="flex-1 flex items-center justify-center text-gray-400">
      <div className="text-center">
        <p className="text-lg mb-1">Select a document or board</p>
        <p className="text-sm">Use the sidebar to navigate</p>
      </div>
    </div>
  )
}
