import React from 'react'
import { Sidebar } from '@/components/sidebar'
import { Header } from '@/components/header'
import { PageRoot, PageView, PageContent } from '@/components/page'
import { AccountingNavigation } from '@/components/accounting/accounting-navigation'

interface AccountingLayoutProps {
  children: React.ReactNode
}

export default function AccountingLayout({ children }: AccountingLayoutProps) {
  return (
    <PageRoot>
      <Sidebar />
      <PageView>
        <Header />
        <AccountingNavigation />
        <PageContent>{children}</PageContent>
      </PageView>
    </PageRoot>
  )
}
