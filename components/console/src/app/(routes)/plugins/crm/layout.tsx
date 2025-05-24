import React from 'react'
import { ReactNode } from 'react'
import { Header } from '@/components/header'
import { Sidebar } from '@/components/sidebar'
import { PageContent, PageRoot, PageView } from '@/components/page'
import { CRMNavigation } from '@/components/crm/crm-navigation'

export default function CRMLayout({ children }: { children: ReactNode }) {
  return (
    <PageRoot>
      <Sidebar />
      <PageView>
        <Header />
        <CRMNavigation />
        <PageContent>{children}</PageContent>
      </PageView>
    </PageRoot>
  )
}
