import React from 'react'
import '@/app/globals.css'
import { QueryProvider } from '@/providers/query-provider'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { Toaster } from '@/components/ui/toast/toaster'
import { LocalizationProvider } from '@/lib/intl'
import { ThemeProvider } from '@/lib/theme'
import ZodSchemaProvider from '@/lib/zod/zod-schema-provider'
import DayjsProvider from '@/providers/dayjs-provider'

export default async function App({ children }: { children: React.ReactNode }) {
  return (
    <LocalizationProvider>
      <QueryProvider>
        <DayjsProvider>
          <ThemeProvider>
            <ZodSchemaProvider>
              {children}
              <Toaster />
            </ZodSchemaProvider>
          </ThemeProvider>
        </DayjsProvider>
        <ReactQueryDevtools initialIsOpen={false} />
      </QueryProvider>
    </LocalizationProvider>
  )
}
