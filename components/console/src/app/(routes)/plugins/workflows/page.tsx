import React from 'react'
import { Metadata } from 'next'
import { WorkflowsOverviewDashboard } from '@/components/workflows/overview/workflows-overview-dashboard'

export const metadata: Metadata = {
  title: 'Workflows - Midaz Console',
  description: 'Business process automation and workflow orchestration'
}

export default function WorkflowsPage() {
  return (
    <div className="p-6">
      <WorkflowsOverviewDashboard />
    </div>
  )
}
