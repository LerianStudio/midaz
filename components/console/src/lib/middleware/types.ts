import { NextRequest, NextResponse } from 'next/server'

export type RouteHandler =
  | (() => NextResponse)
  | (() => Promise<NextResponse>)
  | ((request: NextRequest, ...args: any) => NextResponse)
  | ((request: NextRequest, ...args: any) => Promise<NextResponse>)

export type NextHandler = (err?: any) => Promise<NextResponse>

export type MiddlewareHandler =
  | ((request: NextRequest, next: NextHandler) => NextResponse)
  | ((request: NextRequest, next: NextHandler) => Promise<NextResponse>)
