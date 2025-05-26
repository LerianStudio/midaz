import React from 'react'
import { Metadata } from 'next'
import { ExecutionListTable } from '@/components/workflows/executions/execution-list-table'

export const metadata: Metadata = {
  title: 'Workflow Executions - Workflows',
  description: 'Monitor and manage workflow execution instances'
}

export default function WorkflowExecutionsPage() {
  return <ExecutionListTable />
}
