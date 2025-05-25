import React from 'react'
import { Metadata } from 'next'
import { TemplateEditor } from '@/components/smart-templates/editor/template-editor'

export const metadata: Metadata = {
  title: 'Edit Template - Smart Templates',
  description: 'Edit template content and configuration'
}

interface EditTemplatePageProps {
  params: {
    id: string
  }
}

export default function EditTemplatePage({ params }: EditTemplatePageProps) {
  return <TemplateEditor templateId={params.id} />
}
