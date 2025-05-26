'use client'

import { useState, useCallback, useTransition } from 'react'

type ServerActionResult<T> =
  | {
      success: true
      data: T
    }
  | {
      success: false
      error: string
    }

interface UseServerActionReturn<TInput, TOutput> {
  execute: (input: TInput) => Promise<ServerActionResult<TOutput>>
  isPending: boolean
  error: string | null
  data: TOutput | null
  reset: () => void
}

export function useServerAction<TInput, TOutput>(
  action: (input: TInput) => Promise<ServerActionResult<TOutput>>
): UseServerActionReturn<TInput, TOutput> {
  const [isPending, startTransition] = useTransition()
  const [error, setError] = useState<string | null>(null)
  const [data, setData] = useState<TOutput | null>(null)

  const execute = useCallback(
    async (input: TInput): Promise<ServerActionResult<TOutput>> => {
      return new Promise((resolve) => {
        startTransition(async () => {
          try {
            setError(null)
            const result = await action(input)

            if (result.success) {
              setData(result.data)
              setError(null)
            } else {
              setError(result.error)
              setData(null)
            }

            resolve(result)
          } catch (err) {
            const errorMessage =
              err instanceof Error
                ? err.message
                : 'An unexpected error occurred'
            setError(errorMessage)
            setData(null)
            resolve({
              success: false,
              error: errorMessage
            })
          }
        })
      })
    },
    [action]
  )

  const reset = useCallback(() => {
    setError(null)
    setData(null)
  }, [])

  return {
    execute,
    isPending,
    error,
    data,
    reset
  }
}
