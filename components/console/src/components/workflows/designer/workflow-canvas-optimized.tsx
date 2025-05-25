'use client'

import React, { memo, useCallback, useMemo, useRef, useEffect } from 'react'
import { Node, Edge, NodeProps } from 'reactflow'
import { WorkflowTask } from '@/core/domain/entities/workflow'

// Memoized task node component for performance
export const MemoizedTaskNode = memo<NodeProps>(({ data, selected }) => {
  return (
    <div
      className={`rounded-md border-2 bg-white px-4 py-2 shadow-md ${selected ? 'border-primary' : 'border-stone-400'} min-w-[150px]`}
    >
      <div className="flex items-center">
        <div className="ml-2">
          <div className="text-sm font-bold">{data.label}</div>
          <div className="text-xs text-gray-500">{data.taskType}</div>
        </div>
      </div>
    </div>
  )
})

MemoizedTaskNode.displayName = 'MemoizedTaskNode'

// Memoized edge update function
export const useMemoizedEdgeUpdate = () => {
  return useCallback((oldEdge: Edge, newConnection: Edge) => {
    return { ...oldEdge, ...newConnection }
  }, [])
}

// Memoized node/edge calculation
export const useMemoizedWorkflowData = (
  tasks: WorkflowTask[]
): { nodes: Node[]; edges: Edge[] } => {
  return useMemo(() => {
    if (!tasks || tasks.length === 0) {
      return {
        nodes: [
          {
            id: 'start',
            type: 'input',
            data: { label: 'Start' },
            position: { x: 100, y: 100 },
            style: {
              background: '#4ade80',
              color: 'white',
              border: '1px solid #22c55e',
              borderRadius: '8px'
            }
          }
        ],
        edges: []
      }
    }

    const nodes: Node[] = [
      {
        id: 'start',
        type: 'input',
        data: { label: 'Start' },
        position: { x: 100, y: 100 },
        style: {
          background: '#4ade80',
          color: 'white',
          border: '1px solid #22c55e',
          borderRadius: '8px'
        }
      }
    ]

    const edges: Edge[] = []
    let previousNodeId = 'start'
    let yPosition = 200

    // Convert workflow tasks to nodes
    tasks.forEach((task, index) => {
      const nodeId = task.taskReferenceName || `task_${index}`

      nodes.push({
        id: nodeId,
        type: 'taskNode',
        position: { x: 100, y: yPosition },
        data: {
          label: task.name || task.type,
          taskType: task.type,
          config: task
        }
      })

      // Create edge from previous node
      edges.push({
        id: `${previousNodeId}-${nodeId}`,
        source: previousNodeId,
        target: nodeId,
        type: 'smoothstep'
      })

      previousNodeId = nodeId
      yPosition += 120
    })

    // Add end node
    nodes.push({
      id: 'end',
      type: 'output',
      data: { label: 'End' },
      position: { x: 100, y: yPosition },
      style: {
        background: '#ef4444',
        color: 'white',
        border: '1px solid #dc2626',
        borderRadius: '8px'
      }
    })

    // Connect last task to end
    if (previousNodeId !== 'start') {
      edges.push({
        id: `${previousNodeId}-end`,
        source: previousNodeId,
        target: 'end',
        type: 'smoothstep'
      })
    }

    return { nodes, edges }
  }, [tasks])
}

// Performance monitoring hook
export const usePerformanceMonitor = (componentName: string) => {
  const renderCount = useRef(0)
  const renderStartTime = useRef<number>(0)

  useEffect(() => {
    renderCount.current++
    const renderTime = performance.now() - renderStartTime.current

    if (process.env.NODE_ENV === 'development' && renderTime > 16) {
      console.warn(`${componentName} slow render: ${renderTime.toFixed(2)}ms`)
    }
  })

  renderStartTime.current = performance.now()
}
