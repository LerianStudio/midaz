import React from 'react'
import { Metadata } from 'next'
import { ExecutionDetailViewEnhanced } from '@/components/workflows/executions/execution-detail-view-enhanced'

export const metadata: Metadata = {
  title: 'Execution Details - Workflows',
  description: 'View detailed execution information and timeline'
}

interface ExecutionDetailPageProps {
  params: {
    id: string
  }
}

export default function ExecutionDetailPage({
  params
}: ExecutionDetailPageProps) {
  return <ExecutionDetailViewEnhanced executionId={params.id} />
}
