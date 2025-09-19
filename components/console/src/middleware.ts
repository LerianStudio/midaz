import { NextFetchEvent, NextRequest, NextResponse } from 'next/server'
import { applyMiddleware } from './lib/masterMiddleware'
import apiCorsMiddleware from './middlewares/corsMiddleware'

export async function middleware(request: NextRequest, event: NextFetchEvent) {
  if (request.nextUrl.pathname.startsWith('/_next/image')) {
    const urlParam = request.nextUrl.searchParams.get('url')

    if (urlParam && !urlParam.startsWith('/')) {
      console.warn('🚨 SSRF attempt blocked:', {
        url: request.url,
        remoteUrl: urlParam,
        userAgent: request.headers.get('user-agent'),
        ip: request.headers.get('x-forwarded-for')
      })

      return new NextResponse(
        'Remote image optimization disabled for security',
        {
          status: 403,
          headers: { 'Content-Type': 'text/plain' }
        }
      )
    }
  }

  return applyMiddleware([], () => NextResponse.next(), [apiCorsMiddleware()])(
    request,
    event
  )
}

export const config = {
  matcher: [
    '/((?!api|_next/static|_next/webpack-hmr|favicon.ico).*)',
    '/_next/image/:path*'
  ]
}
