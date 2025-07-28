import { ReactNode } from 'react'
import { Header, Sidebar } from '@lerianstudio/console-layout'
import { PageContent, PageRoot, PageView } from '@/components/page'

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <PageRoot>
      <Sidebar />
      <PageView>
        <Header />
        <PageContent className="p-0">{children}</PageContent>
      </PageView>
    </PageRoot>
  )
}
