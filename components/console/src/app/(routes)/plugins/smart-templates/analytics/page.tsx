import React from 'react'
import { Metadata } from 'next'
import { AnalyticsDashboard } from '@/components/smart-templates/analytics/analytics-dashboard'

export const metadata: Metadata = {
  title: 'Analytics - Smart Templates',
  description: 'Template performance and usage analytics'
}

export default function AnalyticsPage() {
  return <AnalyticsDashboard />
}
