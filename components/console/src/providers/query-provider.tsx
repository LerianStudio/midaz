'use client'

import { ReactNode } from 'react'
import {
  MutationCache,
  QueryClient,
  QueryClientProvider
} from '@tanstack/react-query'
import { useToast } from '@/hooks/use-toast'
import { useIntl } from 'react-intl'

export const QueryProvider = ({ children }: { children: ReactNode }) => {
  const intl = useIntl()
  const { toast } = useToast()

  const queryClient = new QueryClient({
    mutationCache: new MutationCache({
      onError: (error) => {
        toast({
          title: intl.formatMessage({
            id: 'error.query.title',
            defaultMessage: 'Server Error'
          }),
          description: error.message,
          variant: 'destructive'
        })
      }
    })
  })

  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}
