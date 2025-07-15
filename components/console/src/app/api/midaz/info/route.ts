import { MidazInfoDto } from '@/core/application/dto/midaz-info-dto'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextResponse } from 'next/server'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchMidazInfo',
      method: 'GET'
    })
  ],
  async () => {
    const midazInfoDto: MidazInfoDto = {
      version: process.env.VERSION || '1.0.0'
    }
    return NextResponse.json(midazInfoDto)
  }
)
