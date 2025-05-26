import React from 'react'
import { ReactNode } from 'react'
import { Header } from '@/components/header'
import { Sidebar } from '@/components/sidebar'
import { PageContent, PageRoot, PageView } from '@/components/page'
import { FeesNavigation } from '@/components/fees/fees-navigation'

export default function FeesLayout({ children }: { children: ReactNode }) {
  return (
    <PageRoot>
      <Sidebar />
      <PageView>
        <Header />
        <FeesNavigation />
        <PageContent>{children}</PageContent>
      </PageView>
    </PageRoot>
  )
}
