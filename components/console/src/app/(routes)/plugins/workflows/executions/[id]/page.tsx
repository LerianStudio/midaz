import React from 'react'
import { Metadata } from 'next'
import { ExecutionDetailView } from '@/components/workflows/executions/execution-detail-view'

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
  return <ExecutionDetailView executionId={params.id} />
}
