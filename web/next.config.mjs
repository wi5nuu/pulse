/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  transpilePackages: ['y-prosemirror', 'y-protocols'],
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: 'http://localhost:9000/api/:path*',
      },
      {
        source: '/auth/:path*',
        destination: 'http://localhost:9000/auth/:path*',
      },
      {
        source: '/me',
        destination: 'http://localhost:9000/me',
      },
      {
        source: '/invites/:path*',
        destination: 'http://localhost:9000/invites/:path*',
      },
      {
        source: '/users/:path*',
        destination: 'http://localhost:9000/users/:path*',
      },
      {
        source: '/health',
        destination: 'http://localhost:9000/health',
      },
    ]
  },
}

export default nextConfig
