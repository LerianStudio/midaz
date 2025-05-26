import React from 'react'
import { ReactNode } from 'react'
import { Header } from '@/components/header'
import { Sidebar } from '@/components/sidebar'
import { PageContent, PageRoot, PageView } from '@/components/page'
import { SmartTemplatesNavigation } from '@/components/smart-templates/smart-templates-navigation'

export default function SmartTemplatesLayout({
  children
}: {
  children: ReactNode
}) {
  return (
    <PageRoot>
      <Sidebar />
      <PageView>
        <Header />
        <SmartTemplatesNavigation />
        <PageContent>{children}</PageContent>
      </PageView>
    </PageRoot>
  )
}
