import { ReactNode } from 'react'
import { StaticHeader } from '@/components/header'
import { PageContent, PageRoot, PageView } from '@/components/page'

export default async function RootLayout({
  children
}: {
  children: ReactNode
}) {
  return (
    <>
      <PageRoot>
        <PageView>
          <StaticHeader />
          <PageContent>{children}</PageContent>
        </PageView>
      </PageRoot>
    </>
  )
}
