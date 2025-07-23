import '@/app/globals.css'
import React from 'react'
import { nextAuthOptions } from '@/core/infrastructure/next-auth/next-auth-provider'
import {
  AuthRedirect,
  ConsoleLayoutProviders
} from '@lerianstudio/console-layout'

export default async function RootLayout({
  children
}: {
  children: React.ReactNode
}) {
  return (
    <ConsoleLayoutProviders>
      <AuthRedirect nextAuthOptions={nextAuthOptions}>{children}</AuthRedirect>
    </ConsoleLayoutProviders>
  )
}
