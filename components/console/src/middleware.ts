import { NextFetchEvent, NextRequest, NextResponse } from 'next/server'
import { applyMiddleware } from './lib/masterMiddleware'
import apiCorsMiddleware from './middlewares/corsMiddleware'

export async function middleware(request: NextRequest, event: NextFetchEvent) {
  return applyMiddleware([], () => NextResponse.next(), [apiCorsMiddleware()])(
    request,
    event
  )
}
