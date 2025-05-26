import { useEffect, useRef, useCallback } from 'react'
import { usePerformanceStore } from '@/store'

interface PerformanceMetrics {
  renderTime: number
  componentName: string
  props?: Record<string, any>
}

export function usePerformanceMonitor(componentName: string) {
  const renderStartTime = useRef<number>(0)
  const { recordRender } = usePerformanceStore()
  
  // Mark render start
  useEffect(() => {
    renderStartTime.current = performance.now()
  })
  
  // Measure render completion
  useEffect(() => {
    const renderEndTime = performance.now()
    const renderDuration = renderEndTime - renderStartTime.current
    
    // Record metrics
    recordRender(renderDuration)
    
    // Log slow renders in development
    if (process.env.NODE_ENV === 'development' && renderDuration > 16) {
      console.warn(`[Performance] Slow render detected in ${componentName}:`, {
        duration: `${renderDuration.toFixed(2)}ms`,
        threshold: '16ms (60fps)',
      })
    }
    
    // Report to analytics if needed
    if (window.gtag && renderDuration > 50) {
      window.gtag('event', 'slow_render', {
        event_category: 'Performance',
        event_label: componentName,
        value: Math.round(renderDuration),
      })
    }
  })
  
  // Measure specific operations
  const measureOperation = useCallback((
    operationName: string,
    operation: () => void | Promise<void>
  ) => {
    const startTime = performance.now()
    
    const complete = () => {
      const duration = performance.now() - startTime
      
      if (process.env.NODE_ENV === 'development' && duration > 100) {
        console.warn(`[Performance] Slow operation in ${componentName}.${operationName}:`, {
          duration: `${duration.toFixed(2)}ms`,
        })
      }
    }
    
    const result = operation()
    
    if (result instanceof Promise) {
      result.finally(complete)
    } else {
      complete()
    }
    
    return result
  }, [componentName])
  
  return {
    measureOperation,
  }
}

// HOC for automatic performance monitoring
export function withPerformanceMonitoring<P extends object>(
  Component: React.ComponentType<P>,
  componentName?: string
) {
  const displayName = componentName || Component.displayName || Component.name || 'Component'
  
  const WrappedComponent = (props: P) => {
    usePerformanceMonitor(displayName)
    return <Component {...props} />
  }
  
  WrappedComponent.displayName = `withPerformanceMonitoring(${displayName})`
  
  return WrappedComponent
}