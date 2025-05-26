import React from 'react'
import { Metadata } from 'next'
import { MetricsOverviewWidget } from '@/components/smart-templates/analytics/metrics-overview-widget'

export const metadata: Metadata = {
  title: 'Smart Templates - Midaz Console',
  description: 'Template management and report generation system'
}

export default function SmartTemplatesPage() {
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <h1 className="text-2xl font-semibold">Smart Templates</h1>
        <p className="text-muted-foreground">
          Manage templates and generate dynamic reports with live data
          integration
        </p>
      </div>

      <MetricsOverviewWidget />
    </div>
  )
}
