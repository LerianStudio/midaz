import '@/app/globals.css'
import { SidebarProvider } from '@/components/sidebar/primitive'
import { OrganizationProvider } from '@/providers/organization-provider'
import { PermissionProvider } from '@/providers/permission-provider'
import { nextAuthOptions } from '@/core/infrastructure/next-auth/next-auth-provider'
import { getServerSession } from 'next-auth'
import { redirect, RedirectType } from 'next/navigation'
import React from 'react'

export default async function RootLayout({
  children
}: {
  children: React.ReactNode
}) {
  if (process.env.PLUGIN_AUTH_ENABLED === 'true') {
    const session = await getServerSession(nextAuthOptions)

    if (!session) {
      redirect('/signin', RedirectType.replace)
    }

    return (
      <OrganizationProvider>
        <PermissionProvider>
          <SidebarProvider>{children}</SidebarProvider>
        </PermissionProvider>
      </OrganizationProvider>
    )
  }

  return (
    <OrganizationProvider>
      <PermissionProvider>
        <SidebarProvider>{children}</SidebarProvider>
      </PermissionProvider>
    </OrganizationProvider>
  )
}
