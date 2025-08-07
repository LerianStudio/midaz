import { redirect, RedirectType } from 'next/navigation'
import '@/app/globals.css'
import { getServerSession } from 'next-auth/next'
import React from 'react'
import { nextAuthOptions } from '@/core/infrastructure/next-auth/next-auth-provider'

export default async function AuthRoutes({
  children
}: React.PropsWithChildren) {
  if (process.env.PLUGIN_AUTH_ENABLED !== 'true') {
    redirect(`/`, RedirectType.replace)
  }
  const session = await getServerSession(nextAuthOptions)

  if (session?.user) {
    redirect(`/`, RedirectType.replace)
  }

  return <React.Fragment>{children}</React.Fragment>
}
