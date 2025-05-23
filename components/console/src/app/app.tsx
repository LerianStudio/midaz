import React from 'react'
import '@/app/globals.css'
import { QueryProvider } from '@/providers/query-provider'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { Toaster } from 'react-hot-toast'
import { LocalizationProvider } from '@/lib/intl'
import { ThemeProvider } from '@/lib/theme'
import ZodSchemaProvider from '@/lib/zod/zod-schema-provider'
import DayjsProvider from '@/providers/dayjs-provider'

export default async function App({ children }: { children: React.ReactNode }) {
  return (
    <QueryProvider>
      <LocalizationProvider>
        <DayjsProvider>
          <ThemeProvider>
            <ZodSchemaProvider>
              {children}
              <Toaster
                position="top-right"
                containerStyle={{ top: 60, right: 60 }}
              />
            </ZodSchemaProvider>
          </ThemeProvider>
        </DayjsProvider>
      </LocalizationProvider>
      <ReactQueryDevtools initialIsOpen={false} />
    </QueryProvider>
  )
}
