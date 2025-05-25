import React from 'react'
import { Sidebar } from '@/components/sidebar'
import { Header } from '@/components/header'
import { PageRoot, PageView, PageContent } from '@/components/page'

interface AuthLayoutProps {
  children: React.ReactNode
}

export default function AuthLayout({ children }: AuthLayoutProps) {
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