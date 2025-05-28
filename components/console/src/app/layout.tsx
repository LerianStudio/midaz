import 'reflect-metadata'
import React from 'react'
import localFont from 'next/font/local'
import NextAuthSessionProvider from '@/providers/next-auth-session-provider'
import { Metadata } from 'next'
import { getMetadata } from '../../services/configs/application-config'
import App from './app'

const inter = localFont({
  src: '../../public/fonts/inter-variable.woff2',
  variable: '--font-inter',
  display: 'swap',
  weight: '100 900'
})

export default async function RootLayout({
  children
}: {
  children: React.ReactNode
}) {
  return (
    <html suppressHydrationWarning className={inter.variable}>
      <body suppressHydrationWarning className="font-sans">
        <NextAuthSessionProvider>
          <App>{children}</App>
        </NextAuthSessionProvider>
      </body>
    </html>
  )
}

export async function generateMetadata(props: {}): Promise<Metadata> {
  const { title, icons, description } = await getMetadata()

  return {
    title: title,
    icons: icons,
    description: description,
    ...props
  }
}
