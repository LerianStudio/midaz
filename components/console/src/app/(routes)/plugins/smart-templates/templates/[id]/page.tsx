import React from 'react'
import { Metadata } from 'next'
import { TemplateDetailView } from '@/components/smart-templates/templates/template-detail-view'

export const metadata: Metadata = {
  title: 'Template Details - Smart Templates',
  description: 'View and manage template details'
}

interface TemplateDetailPageProps {
  params: {
    id: string
  }
}

export default function TemplateDetailPage({
  params
}: TemplateDetailPageProps) {
  return <TemplateDetailView templateId={params.id} />
}
