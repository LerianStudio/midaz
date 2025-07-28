import { Sidebar, Header } from '@lerianstudio/console-layout'
import { PageContent, PageRoot, PageView } from '@/components/page'

export default function RootLayout({
  children
}: {
  children: React.ReactNode
}) {
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
