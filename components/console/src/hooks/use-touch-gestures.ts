'use client'

import { useEffect, useRef, useCallback } from 'react'

interface SwipeHandlers {
  onSwipeLeft?: () => void
  onSwipeRight?: () => void
  onSwipeUp?: () => void
  onSwipeDown?: () => void
}

interface TouchPoint {
  x: number
  y: number
  time: number
}

export function useTouchGestures<T extends HTMLElement>(
  handlers: SwipeHandlers,
  options = {
    minSwipeDistance: 50,
    maxSwipeTime: 300,
    preventScroll: false
  }
) {
  const ref = useRef<T>(null)
  const touchStart = useRef<TouchPoint | null>(null)

  const handleTouchStart = useCallback(
    (e: TouchEvent) => {
      const touch = e.touches[0]
      touchStart.current = {
        x: touch.clientX,
        y: touch.clientY,
        time: Date.now()
      }

      if (options.preventScroll) {
        e.preventDefault()
      }
    },
    [options.preventScroll]
  )

  const handleTouchEnd = useCallback(
    (e: TouchEvent) => {
      if (!touchStart.current) return

      const touch = e.changedTouches[0]
      const deltaX = touch.clientX - touchStart.current.x
      const deltaY = touch.clientY - touchStart.current.y
      const deltaTime = Date.now() - touchStart.current.time

      // Check if it's a valid swipe
      if (deltaTime > options.maxSwipeTime) {
        touchStart.current = null
        return
      }

      const absX = Math.abs(deltaX)
      const absY = Math.abs(deltaY)

      // Horizontal swipe
      if (absX > absY && absX > options.minSwipeDistance) {
        if (deltaX > 0 && handlers.onSwipeRight) {
          handlers.onSwipeRight()
        } else if (deltaX < 0 && handlers.onSwipeLeft) {
          handlers.onSwipeLeft()
        }
      }
      // Vertical swipe
      else if (absY > absX && absY > options.minSwipeDistance) {
        if (deltaY > 0 && handlers.onSwipeDown) {
          handlers.onSwipeDown()
        } else if (deltaY < 0 && handlers.onSwipeUp) {
          handlers.onSwipeUp()
        }
      }

      touchStart.current = null
    },
    [handlers, options.minSwipeDistance, options.maxSwipeTime]
  )

  const handleTouchMove = useCallback(
    (e: TouchEvent) => {
      if (options.preventScroll && touchStart.current) {
        e.preventDefault()
      }
    },
    [options.preventScroll]
  )

  useEffect(() => {
    const element = ref.current
    if (!element) return

    element.addEventListener('touchstart', handleTouchStart, {
      passive: !options.preventScroll
    })
    element.addEventListener('touchend', handleTouchEnd, { passive: true })
    element.addEventListener('touchmove', handleTouchMove, {
      passive: !options.preventScroll
    })

    return () => {
      element.removeEventListener('touchstart', handleTouchStart)
      element.removeEventListener('touchend', handleTouchEnd)
      element.removeEventListener('touchmove', handleTouchMove)
    }
  }, [handleTouchStart, handleTouchEnd, handleTouchMove, options.preventScroll])

  return ref
}

// Hook for pinch-to-zoom gestures
export function usePinchToZoom<T extends HTMLElement>(
  onZoom: (scale: number) => void,
  options = {
    minScale: 0.5,
    maxScale: 3
  }
) {
  const ref = useRef<T>(null)
  const initialDistance = useRef<number | null>(null)
  const currentScale = useRef(1)

  const handleTouchStart = useCallback((e: TouchEvent) => {
    if (e.touches.length === 2) {
      const touch1 = e.touches[0]
      const touch2 = e.touches[1]
      const distance = Math.hypot(
        touch2.clientX - touch1.clientX,
        touch2.clientY - touch1.clientY
      )
      initialDistance.current = distance
    }
  }, [])

  const handleTouchMove = useCallback(
    (e: TouchEvent) => {
      if (e.touches.length === 2 && initialDistance.current) {
        const touch1 = e.touches[0]
        const touch2 = e.touches[1]
        const distance = Math.hypot(
          touch2.clientX - touch1.clientX,
          touch2.clientY - touch1.clientY
        )

        const scale = distance / initialDistance.current
        const newScale = Math.min(
          Math.max(currentScale.current * scale, options.minScale),
          options.maxScale
        )

        if (newScale !== currentScale.current) {
          currentScale.current = newScale
          onZoom(newScale)
        }
      }
    },
    [onZoom, options.minScale, options.maxScale]
  )

  const handleTouchEnd = useCallback(() => {
    initialDistance.current = null
  }, [])

  useEffect(() => {
    const element = ref.current
    if (!element) return

    element.addEventListener('touchstart', handleTouchStart, { passive: true })
    element.addEventListener('touchmove', handleTouchMove, { passive: true })
    element.addEventListener('touchend', handleTouchEnd, { passive: true })

    return () => {
      element.removeEventListener('touchstart', handleTouchStart)
      element.removeEventListener('touchmove', handleTouchMove)
      element.removeEventListener('touchend', handleTouchEnd)
    }
  }, [handleTouchStart, handleTouchMove, handleTouchEnd])

  return ref
}
