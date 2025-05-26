import React from 'react'
import { Metadata } from 'next'
import { SmartTemplatesDemoWizard } from '@/components/smart-templates/smart-templates-demo-wizard'

export const metadata: Metadata = {
  title: 'Demo - Smart Templates',
  description: 'Interactive demo of Smart Templates features'
}

export default function SmartTemplatesDemoPage() {
  return <SmartTemplatesDemoWizard />
}
