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

    if (!isDev) {
      return next()
    }

    const pathname = req.nextUrl.pathname

    if (!checkWhitelist(pathname, ['/api/*any'])) {
      return next()
    }

    const origin = req.headers.get('origin')

    const nextHeaderList = nextConfig.headers ? await nextConfig.headers() : []

    const endPointHeaders = nextHeaderList.find((header: any) =>
      header.source.includes('/api')
    )

    if (endPointHeaders) {
      endPointHeaders.headers.forEach((header: any) => {
        response?.headers.set(header.key, header.value)
      })
    }

    response?.headers.set('Access-Control-Allow-Origin', origin || '*')

    return next()
  }
}
