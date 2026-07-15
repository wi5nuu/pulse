import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'

const publicPaths = ['/login', '/register', '/auth', '/invite', '/invites']

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl
  if (publicPaths.some((p) => pathname.startsWith(p))) {
    return NextResponse.next()
  }
  const token = request.cookies.get('pulse_refresh')?.value
  if (!token) {
    return NextResponse.redirect(new URL('/login', request.url))
  }
  return NextResponse.next()
}

export const config = {
  matcher: ['/((?!_next|favicon.ico).*)'],
}
