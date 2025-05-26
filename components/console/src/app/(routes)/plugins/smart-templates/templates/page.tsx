import React from 'react'
import { Metadata } from 'next'
import { TemplateListView } from '@/components/smart-templates/templates/template-list-view'

export const metadata: Metadata = {
  title: 'Templates - Smart Templates - Midaz Console',
  description: 'Manage template library and configurations'
}

export default function TemplatesPage() {
  return <TemplateListView />
}
