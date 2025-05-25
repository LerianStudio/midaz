import React from 'react'
import { Metadata } from 'next'
import { WorkflowCanvas } from '@/components/workflows/designer/workflow-canvas'

export const metadata: Metadata = {
  title: 'Workflow Designer - Workflows',
  description: 'Visual workflow designer with drag-and-drop interface'
}

interface WorkflowDesignerPageProps {
  params: {
    id: string
  }
}

export default function WorkflowDesignerPage({
  params
}: WorkflowDesignerPageProps) {
  return (
    <div className="h-screen overflow-hidden">
      <WorkflowCanvas />
    </div>
  )
}
