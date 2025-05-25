import React from 'react'
import { Sidebar } from '@/components/sidebar'
import { Header } from '@/components/header'
import { PageRoot, PageView, PageContent } from '@/components/page'
import { WorkflowsNavigation } from '@/components/workflows/workflows-navigation'

interface WorkflowsLayoutProps {
  children: React.ReactNode
}

export default function WorkflowsLayout({ children }: WorkflowsLayoutProps) {
  return (
    <PageRoot>
      <Sidebar />
      <PageView>
        <Header />
        <WorkflowsNavigation />
        <PageContent>{children}</PageContent>
      </PageView>
    </PageRoot>
  )
}
