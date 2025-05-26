'use client'

import { useEffect, useRef } from 'react'

interface UseAutoRefreshOptions {
  enabled?: boolean
  interval?: number
  onRefresh: () => void | Promise<void>
}

export function useAutoRefresh({
  enabled = true,
  interval = 5000,
  onRefresh
}: UseAutoRefreshOptions) {
  const intervalRef = useRef<NodeJS.Timeout | null>(null)

  useEffect(() => {
    if (!enabled) {
      if (intervalRef.current) {
        clearInterval(intervalRef.current)
        intervalRef.current = null
      }
      return
    }

    // Initial fetch
    onRefresh()

    // Set up interval
    intervalRef.current = setInterval(() => {
      onRefresh()
    }, interval)

    // Cleanup
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current)
        intervalRef.current = null
      }
    }
  }, [enabled, interval, onRefresh])

  const manualRefresh = () => {
    onRefresh()
  }

  return { manualRefresh }
}
