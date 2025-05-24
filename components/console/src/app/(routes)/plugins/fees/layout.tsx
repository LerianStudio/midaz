import React from 'react'
import { PageRoot } from '@/components/page'
import { FeesNavigation } from '@/components/fees/fees-navigation'

export default function FeesLayout({
  children
}: {
  children: React.ReactNode
}) {
  return (
    <PageRoot className="flex flex-col gap-6">
      <FeesNavigation />
      <div className="flex-1">{children}</div>
    </PageRoot>
  )
}
