import React from 'react'
import { Metadata } from 'next'
import { TemplateCreationWizard } from '@/components/smart-templates/templates/template-creation-wizard'

export const metadata: Metadata = {
  title: 'Create Template - Smart Templates',
  description: 'Create a new document template'
}

export default function CreateTemplatePage() {
  return <TemplateCreationWizard />
}
