import { MidazInfoDto } from '@/core/application/dto/midaz-info-dto'
import { NextRequest, NextResponse } from 'next/server'

export async function GET(request: NextRequest) {
  const midazInfo: MidazInfoDto = {
    version: process.env.VERSION!
  }

  return NextResponse.json(midazInfo)
}
