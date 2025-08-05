import { NextFetchEvent, NextRequest, NextResponse } from 'next/server'

export type RouteHandler =
  | (() => NextResponse)
  | (() => Promise<NextResponse>)
  | ((request: NextRequest) => NextResponse)
  | ((request: NextRequest) => Promise<NextResponse>)
  | ((request: NextRequest, event: NextFetchEvent) => NextResponse)
  | ((request: NextRequest, event: NextFetchEvent) => Promise<NextResponse>)

export type NextHandler = (err?: any) => Promise<NextResponse>

export type MiddlewareHandler =
  | ((
      request: NextRequest,
      event: NextFetchEvent,
      next: NextHandler,
      response?: NextResponse
    ) => NextResponse)
  | ((
      request: NextRequest,
      event: NextFetchEvent,
      next: NextHandler,
      response?: NextResponse
    ) => Promise<NextResponse>)
