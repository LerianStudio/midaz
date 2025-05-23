import { ReactNode } from 'react'
import { Header } from '@/components/header'
import { Sidebar } from '@/components/sidebar'
import { PageContent, PageRoot, PageView } from '@/components/page'

export default function RootLayout({ children }: { children: ReactNode }) {
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
