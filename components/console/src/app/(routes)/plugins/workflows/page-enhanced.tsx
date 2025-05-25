import React from 'react'
import { Metadata } from 'next'
import { WorkflowsOverviewDashboard } from '@/components/workflows/overview/workflows-overview-dashboard'
import { WorkflowErrorBoundaryWrapper } from '@/components/workflows/error-boundary'

export const metadata: Metadata = {
  title: 'Workflows - Midaz Console',
  description: 'Business process automation and workflow orchestration'
}

export default function WorkflowsPage() {
  return (
    <WorkflowErrorBoundaryWrapper
      onError={(error, errorInfo) => {
        // Log to error tracking service
        console.error('Workflows page error:', error, errorInfo)
      }}
    >
      <div className="p-6">
        <WorkflowsOverviewDashboard />
      </div>
    </WorkflowErrorBoundaryWrapper>
  )
}
