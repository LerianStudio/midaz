'use client'

import { ReactNode } from 'react'
import { OnboardFormProvider } from './onboard-form-provider'

export default function RootLayout({ children }: { children: ReactNode }) {
  return <OnboardFormProvider>{children}</OnboardFormProvider>
}
