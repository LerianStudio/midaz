'use client'

import { TransactionProvider } from './transaction-form-provider'

export default function RootLayout({
  children
}: {
  children: React.ReactNode
}) {
  return <TransactionProvider>{children}</TransactionProvider>
}
