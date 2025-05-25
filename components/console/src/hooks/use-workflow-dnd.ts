import { useState, useCallback, useRef, useEffect } from 'react'
import { TaskType } from '@/core/domain/entities/workflow'

interface DragState {
  isDragging: boolean
  draggedTaskType: TaskType | null
  dragPosition: { x: number; y: number } | null
  isOverDropZone: boolean
}

interface UseWorkflowDndOptions {
  onDrop?: (taskType: TaskType, position: { x: number; y: number }) => void
  enabled?: boolean
}

export function useWorkflowDnd(options: UseWorkflowDndOptions = {}) {
  const { onDrop, enabled = true } = options
  const [dragState, setDragState] = useState<DragState>({
    isDragging: false,
    draggedTaskType: null,
    dragPosition: null,
    isOverDropZone: false
  })

  const dragGhostRef = useRef<HTMLDivElement | null>(null)
  const touchStartPosRef = useRef<{ x: number; y: number } | null>(null)
  const touchTimeoutRef = useRef<NodeJS.Timeout | null>(null)

  // Clean up function for drag ghost
  const cleanupDragGhost = useCallback(() => {
    if (dragGhostRef.current) {
      document.body.removeChild(dragGhostRef.current)
      dragGhostRef.current = null
    }
  }, [])

  // Handle drag start
  const handleDragStart = useCallback(
    (event: React.DragEvent, taskType: TaskType) => {
      if (!enabled) return

      // Set drag data
      event.dataTransfer.setData('application/reactflow', taskType)
      event.dataTransfer.effectAllowed = 'copy'

      // Create custom drag image
      const dragGhost = document.createElement('div')
      dragGhost.className = 'drag-ghost'
      dragGhost.style.cssText = `
        position: fixed;
        top: -1000px;
        left: -1000px;
        padding: 8px 16px;
        background: hsl(var(--primary));
        color: hsl(var(--primary-foreground));
        border-radius: 6px;
        font-size: 14px;
        font-weight: 500;
        box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
        pointer-events: none;
        z-index: 9999;
      `
      dragGhost.textContent = taskType
      document.body.appendChild(dragGhost)
      dragGhostRef.current = dragGhost

      // Set custom drag image
      event.dataTransfer.setDragImage(dragGhost, 0, 0)

      setDragState({
        isDragging: true,
        draggedTaskType: taskType,
        dragPosition: { x: event.clientX, y: event.clientY },
        isOverDropZone: false
      })
    },
    [enabled]
  )

  // Handle drag end
  const handleDragEnd = useCallback(() => {
    cleanupDragGhost()
    setDragState({
      isDragging: false,
      draggedTaskType: null,
      dragPosition: null,
      isOverDropZone: false
    })
  }, [cleanupDragGhost])

  // Handle drag over
  const handleDragOver = useCallback(
    (event: React.DragEvent) => {
      if (!enabled || !dragState.isDragging) return

      event.preventDefault()
      event.dataTransfer.dropEffect = 'copy'

      setDragState((prev) => ({
        ...prev,
        dragPosition: { x: event.clientX, y: event.clientY },
        isOverDropZone: true
      }))
    },
    [enabled, dragState.isDragging]
  )

  // Handle drag leave
  const handleDragLeave = useCallback(() => {
    setDragState((prev) => ({
      ...prev,
      isOverDropZone: false
    }))
  }, [])

  // Handle drop
  const handleDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault()

      const taskType = event.dataTransfer.getData(
        'application/reactflow'
      ) as TaskType
      if (!taskType || !onDrop) return

      const bounds = event.currentTarget.getBoundingClientRect()
      const position = {
        x: event.clientX - bounds.left,
        y: event.clientY - bounds.top
      }

      onDrop(taskType, position)
      handleDragEnd()
    },
    [onDrop, handleDragEnd]
  )

  // Touch event handlers for mobile support
  const handleTouchStart = useCallback(
    (event: React.TouchEvent, taskType: TaskType) => {
      if (!enabled) return

      const touch = event.touches[0]
      touchStartPosRef.current = { x: touch.clientX, y: touch.clientY }

      // Start long press detection
      touchTimeoutRef.current = setTimeout(() => {
        // Trigger haptic feedback if available
        if ('vibrate' in navigator) {
          navigator.vibrate(50)
        }

        setDragState({
          isDragging: true,
          draggedTaskType: taskType,
          dragPosition: { x: touch.clientX, y: touch.clientY },
          isOverDropZone: false
        })
      }, 500) // 500ms long press
    },
    [enabled]
  )

  const handleTouchMove = useCallback(
    (event: React.TouchEvent) => {
      if (!dragState.isDragging) return

      const touch = event.touches[0]
      setDragState((prev) => ({
        ...prev,
        dragPosition: { x: touch.clientX, y: touch.clientY }
      }))

      // Check if over drop zone
      const element = document.elementFromPoint(touch.clientX, touch.clientY)
      const isOverCanvas = element?.closest('.react-flow')
      setDragState((prev) => ({
        ...prev,
        isOverDropZone: !!isOverCanvas
      }))
    },
    [dragState.isDragging]
  )

  const handleTouchEnd = useCallback(
    (event: React.TouchEvent) => {
      // Clear long press timeout
      if (touchTimeoutRef.current) {
        clearTimeout(touchTimeoutRef.current)
        touchTimeoutRef.current = null
      }

      if (!dragState.isDragging || !dragState.draggedTaskType) return

      // Get the element at touch position
      const touch = event.changedTouches[0]
      const element = document.elementFromPoint(touch.clientX, touch.clientY)
      const canvasElement = element?.closest('.react-flow')

      if (canvasElement && onDrop) {
        const bounds = canvasElement.getBoundingClientRect()
        const position = {
          x: touch.clientX - bounds.left,
          y: touch.clientY - bounds.top
        }

        onDrop(dragState.draggedTaskType, position)
      }

      handleDragEnd()
    },
    [dragState.isDragging, dragState.draggedTaskType, onDrop, handleDragEnd]
  )

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      cleanupDragGhost()
      if (touchTimeoutRef.current) {
        clearTimeout(touchTimeoutRef.current)
      }
    }
  }, [cleanupDragGhost])

  return {
    dragState,
    handlers: {
      onDragStart: handleDragStart,
      onDragEnd: handleDragEnd,
      onDragOver: handleDragOver,
      onDragLeave: handleDragLeave,
      onDrop: handleDrop,
      onTouchStart: handleTouchStart,
      onTouchMove: handleTouchMove,
      onTouchEnd: handleTouchEnd
    }
  }
}

// Helper hook for drop zone indicators
export function useDropZoneIndicator(isActive: boolean) {
  const [showIndicator, setShowIndicator] = useState(false)

  useEffect(() => {
    if (isActive) {
      setShowIndicator(true)
    } else {
      // Delay hiding to allow for smooth transitions
      const timeout = setTimeout(() => setShowIndicator(false), 200)
      return () => clearTimeout(timeout)
    }
  }, [isActive])

  return showIndicator
}
