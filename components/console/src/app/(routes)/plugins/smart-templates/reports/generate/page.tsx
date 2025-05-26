import React from 'react'
import { Metadata } from 'next'
import { ReportGenerationWizard } from '@/components/smart-templates/reports/report-generation-wizard'

export const metadata: Metadata = {
  title: 'Generate Report - Smart Templates',
  description: 'Generate a new report from template'
}

export default function GenerateReportPage() {
  return <ReportGenerationWizard />
}
