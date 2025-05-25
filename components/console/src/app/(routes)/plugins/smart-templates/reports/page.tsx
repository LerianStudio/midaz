import React from 'react'
import { Metadata } from 'next'
import { ReportMonitoringDashboard } from '@/components/smart-templates/reports/report-monitoring-dashboard'

export const metadata: Metadata = {
  title: 'Reports - Smart Templates',
  description: 'Monitor and manage report generation'
}

export default function ReportsPage() {
  return <ReportMonitoringDashboard />
}
