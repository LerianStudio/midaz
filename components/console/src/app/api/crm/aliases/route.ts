import { NextResponse } from 'next/server'
import { getAllAliases } from '@/app/actions/crm'

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const organizationId = searchParams.get('organizationId')
    const limit = Number(searchParams.get('limit')) || 100
    const page = Number(searchParams.get('page')) || 1

    if (!organizationId) {
      return NextResponse.json(
        { error: 'organizationId is required' },
        { status: 400 }
      )
    }

    const result = await getAllAliases({ organizationId, limit, page })

    if (result.success && result.data) {
      return NextResponse.json(result.data)
    } else {
      return NextResponse.json(
        { error: result.error || 'Failed to fetch aliases' },
        { status: 500 }
      )
    }
  } catch (error: any) {
    console.error('Error fetching aliases:', error)
    return NextResponse.json(
      { error: error.message || 'Internal server error' },
      { status: 500 }
    )
  }
}