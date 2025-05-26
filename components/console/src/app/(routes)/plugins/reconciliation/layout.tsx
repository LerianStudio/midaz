import React from 'react'
import { Sidebar } from '@/components/sidebar'
import { Header } from '@/components/header'
import { PageRoot, PageView, PageContent } from '@/components/page'
import { ReconciliationNavigation } from '@/components/reconciliation/reconciliation-navigation'

interface ReconciliationLayoutProps {
  children: React.ReactNode
}

export default function ReconciliationLayout({
  children
}: ReconciliationLayoutProps) {
  return (
    <PageRoot>
      <Sidebar />
      <PageView>
        <Header />
        <ReconciliationNavigation />
        <PageContent>{children}</PageContent>
      </PageView>
    </PageRoot>
  )
}
