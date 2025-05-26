'use client'

import { useEffect, useRef, useState, useCallback } from 'react'

interface UseIntersectionObserverOptions {
  root?: Element | null
  rootMargin?: string
  threshold?: number | number[]
  freezeOnceVisible?: boolean
}

export function useIntersectionObserver(
  options: UseIntersectionObserverOptions = {}
): [(node: Element | null) => void, IntersectionObserverEntry | undefined] {
  const {
    root = null,
    rootMargin = '0px',
    threshold = 0,
    freezeOnceVisible = false
  } = options

  const [entry, setEntry] = useState<IntersectionObserverEntry>()
  const [node, setNode] = useState<Element | null>(null)

  const frozen = useRef(false)

  const updateEntry = useCallback(
    ([entry]: IntersectionObserverEntry[]): void => {
      if (frozen.current && freezeOnceVisible) return
      setEntry(entry)
      if (entry.isIntersecting && freezeOnceVisible) {
        frozen.current = true
      }
    },
    [freezeOnceVisible]
  )

  useEffect(() => {
    if (!node) return

    const observer = new IntersectionObserver(updateEntry, {
      root,
      rootMargin,
      threshold
    })

    observer.observe(node)

    return () => observer.disconnect()
  }, [node, root, rootMargin, threshold, updateEntry])

  const ref = useCallback((node: Element | null) => {
    setNode(node)
  }, [])

  return [ref, entry]
}

// Hook for lazy loading components
export function useLazyLoad(options: UseIntersectionObserverOptions = {}): {
  ref: (node: Element | null) => void
  isVisible: boolean
  hasBeenVisible: boolean
} {
  const [ref, entry] = useIntersectionObserver({
    ...options,
    freezeOnceVisible: true
  })

  const isVisible = !!entry?.isIntersecting
  const hasBeenVisible = useRef(false)

  useEffect(() => {
    if (isVisible) {
      hasBeenVisible.current = true
    }
  }, [isVisible])

  return {
    ref,
    isVisible,
    hasBeenVisible: hasBeenVisible.current
  }
}

// Hook for virtual scrolling
export function useVirtualScroll<T>({
  items,
  itemHeight,
  containerHeight,
  overscan = 3
}: {
  items: T[]
  itemHeight: number
  containerHeight: number
  overscan?: number
}) {
  const [scrollTop, setScrollTop] = useState(0)

  const startIndex = Math.max(0, Math.floor(scrollTop / itemHeight) - overscan)
  const endIndex = Math.min(
    items.length - 1,
    Math.ceil((scrollTop + containerHeight) / itemHeight) + overscan
  )

  const visibleItems = items.slice(startIndex, endIndex + 1)
  const totalHeight = items.length * itemHeight
  const offsetY = startIndex * itemHeight

  const handleScroll = useCallback((e: React.UIEvent<HTMLElement>) => {
    setScrollTop(e.currentTarget.scrollTop)
  }, [])

  return {
    visibleItems,
    totalHeight,
    offsetY,
    handleScroll,
    startIndex,
    endIndex
  }
}
