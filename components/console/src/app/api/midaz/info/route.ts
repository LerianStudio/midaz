import { MidazInfoDto } from '@/core/application/dto/midaz-info-dto'
import { NextResponse } from 'next/server'

export async function GET() {
  const midazInfoDto: MidazInfoDto = {
    version: process.env.VERSION || '1.0.0'
  }

  return NextResponse.json(midazInfoDto)
}
