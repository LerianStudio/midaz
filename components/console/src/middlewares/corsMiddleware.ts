import { NextHandler } from '@/lib/masterMiddleware'
import { checkWhitelist } from '@/lib/masterMiddleware/checkWhitelist'
import { NextFetchEvent, NextRequest, NextResponse } from 'next/server'
import nextConfig from '../../next.config.mjs'

export default function apiCorsMiddleware() {
  return async (
    req: NextRequest,
    event: NextFetchEvent,
    next: NextHandler,
    response?: NextResponse
  ) => {
    const isDev = process.env.NODE_ENV === 'development'

    // Skip tampering with cors middleware in production
    if (!isDev) {
      return next()
    }

    const pathname = req.nextUrl.pathname

    // Skip if not an api endpoint
    if (!checkWhitelist(pathname, ['/api/*any'])) {
      return next()
    }

    // Get origin from request
    const origin = req.headers.get('origin')

    // Get next.config.mjs headers
    const nextHeaderList = nextConfig.headers ? await nextConfig.headers() : []

    // Find api endpoint headers
    const endPointHeaders = nextHeaderList.find((header: any) =>
      header.source.includes('/api')
    )

    // Set api endpoint headers
    if (endPointHeaders) {
      endPointHeaders.headers.forEach((header: any) => {
        response?.headers.set(header.key, header.value)
      })
    }

    // Set origin the same as the request
    response?.headers.set('Access-Control-Allow-Origin', origin || '*')

    return next()
  }
}
