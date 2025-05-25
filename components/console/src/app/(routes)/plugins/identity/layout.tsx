import React from 'react'
import { Sidebar } from '@/components/sidebar'
import { Header } from '@/components/header'
import { PageRoot, PageView, PageContent } from '@/components/page'

interface IdentityLayoutProps {
  children: React.ReactNode
}

export default function IdentityLayout({ children }: IdentityLayoutProps) {
  return (
    <PageRoot>
      <Sidebar />
      <PageView>
        <Header />
        <PageContent>{children}</PageContent>
      </PageView>
    </PageRoot>
  )
}