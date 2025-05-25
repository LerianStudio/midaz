'use client'

import { useCallback, useState, useEffect, useMemo } from 'react'
import ReactFlow, {
  MiniMap,
  Controls,
  Background,
  useNodesState,
  useEdgesState,
  addEdge,
  Connection,
  Edge,
  Node,
  BackgroundVariant,
  ReactFlowProvider
} from 'reactflow'
import 'reactflow/dist/style.css'
import { TaskNodeComponent } from './task-node-components'
import { TaskPalette } from './task-palette'
import { TaskConfigurationPanel } from './task-configuration-panel'
import { WorkflowMetadataEditor } from './workflow-metadata-editor'
import { CanvasControls } from './canvas-controls'
import { WorkflowValidator } from './workflow-validator'
import { WorkflowImportExport } from '../library/workflow-import-export'
import {
  Workflow,
  WorkflowTask,
  TaskType
} from '@/core/domain/entities/workflow'
import { useMediaQuery } from '@/hooks/use-media-query'
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { Menu, X } from 'lucide-react'
import { useToast } from '@/hooks/use-toast'
import { useWorkflowDnd, useDropZoneIndicator } from '@/hooks/use-workflow-dnd'
import { cn } from '@/lib/utils'

const nodeTypes = {
  taskNode: TaskNodeComponent
}

interface WorkflowCanvasProps {
  workflow?: Workflow
  onWorkflowChange?: (workflow: Workflow) => void
  readonly?: boolean
}

const initialNodes: Node[] = [
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

const initialEdges: Edge[] = []

function WorkflowCanvasInner({
  workflow,
  onWorkflowChange,
  readonly = false
}: WorkflowCanvasProps) {
  const { toast } = useToast()
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)
  const [selectedNode, setSelectedNode] = useState<Node | null>(null)
  const [showMetadataEditor, setShowMetadataEditor] = useState(false)
  const [isInitialized, setIsInitialized] = useState(false)
  const [showTaskPalette, setShowTaskPalette] = useState(false)
  const [showConfigPanel, setShowConfigPanel] = useState(false)
  const [showValidationDialog, setShowValidationDialog] = useState(false)
  const [showImportDialog, setShowImportDialog] = useState(false)
  const [showExportDialog, setShowExportDialog] = useState(false)
  const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false)
  const [reactFlowInstance, setReactFlowInstance] = useState<any>(null)

  // Media queries for responsive design
  const isMobile = useMediaQuery('(max-width: 768px)')
  const isTablet = useMediaQuery('(max-width: 1024px)')

  // Convert workflow tasks to nodes and edges when workflow changes
  useEffect(() => {
    if (
      !workflow ||
      !workflow.tasks ||
      workflow.tasks.length === 0 ||
      isInitialized
    )
      return

    const newNodes: Node[] = [
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

    const newEdges: Edge[] = []
    let previousNodeId = 'start'
    let yPosition = 200

    // Convert workflow tasks to nodes
    workflow.tasks.forEach((task, index) => {
      const nodeId = task.taskReferenceName || `task_${index}`

      newNodes.push({
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
      if (previousNodeId) {
        newEdges.push({
          id: `${previousNodeId}-${nodeId}`,
          source: previousNodeId,
          target: nodeId,
          type: 'smoothstep'
        })
      }

      previousNodeId = nodeId
      yPosition += 120
    })

    // Add end node
    const endNodeId = 'end'
    newNodes.push({
      id: endNodeId,
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
    if (previousNodeId && previousNodeId !== 'start') {
      newEdges.push({
        id: `${previousNodeId}-${endNodeId}`,
        source: previousNodeId,
        target: endNodeId,
        type: 'smoothstep'
      })
    }

    setNodes(newNodes)
    setEdges(newEdges)
    setIsInitialized(true)
  }, [workflow, isInitialized, setNodes, setEdges])

  // Convert nodes back to workflow tasks and update workflow
  const updateWorkflowFromCanvas = useCallback(() => {
    if (!onWorkflowChange || !workflow) return

    // Extract task nodes (exclude start and end nodes) - currently not used directly
    // as we traverse from start node to maintain order

    // Sort nodes by their connections to maintain order
    const sortedTasks: WorkflowTask[] = []
    const nodeMap = new Map(nodes.map((n) => [n.id, n]))
    const visited = new Set<string>()

    const traverse = (nodeId: string) => {
      if (visited.has(nodeId)) return
      visited.add(nodeId)

      const node = nodeMap.get(nodeId)
      if (node && node.type === 'taskNode' && node.data.config) {
        sortedTasks.push(node.data.config)
      }

      // Find connected nodes
      const outgoingEdges = edges.filter((e) => e.source === nodeId)
      outgoingEdges.forEach((edge) => traverse(edge.target))
    }

    // Start traversal from start node
    traverse('start')

    // Update workflow with new tasks
    const updatedWorkflow: Workflow = {
      ...workflow,
      tasks: sortedTasks,
      updatedAt: new Date().toISOString()
    }

    onWorkflowChange(updatedWorkflow)
    setHasUnsavedChanges(true)
  }, [nodes, edges, workflow, onWorkflowChange])

  const onConnect = useCallback(
    (params: Connection) => {
      setEdges((eds) => addEdge(params, eds))
      // Update workflow after connection change
      setTimeout(updateWorkflowFromCanvas, 100)
    },
    [setEdges, updateWorkflowFromCanvas]
  )

  const onNodeClick = useCallback((_event: React.MouseEvent, node: Node) => {
    setSelectedNode(node)
  }, [])

  const onPaneClick = useCallback(() => {
    setSelectedNode(null)
  }, [])

  // Create node from drag and drop
  const createNodeFromDrop = useCallback(
    (taskType: TaskType, position: { x: number; y: number }) => {
      // Convert screen position to flow position
      const flowPosition = reactFlowInstance?.project(position) || position

      // Find optimal position based on existing nodes
      const nodePositions = nodes.map((n) => ({
        x: n.position.x,
        y: n.position.y
      }))
      const optimalPosition = findOptimalNodePosition(
        flowPosition,
        nodePositions
      )

      const newNode: Node = {
        id: `${taskType.toLowerCase()}_${Date.now()}`,
        type: 'taskNode',
        position: optimalPosition,
        data: {
          label: taskType,
          taskType: taskType,
          config: {
            name: `${taskType.toLowerCase()}_task`,
            taskReferenceName: `${taskType.toLowerCase()}_${Date.now()}`,
            type: taskType,
            inputParameters: {}
          }
        }
      }

      setNodes((nds) => nds.concat(newNode))

      // Add animation effect
      toast({
        title: 'Task added',
        description: `${taskType} task has been added to the workflow`,
        duration: 2000
      })

      // Update workflow after adding node
      setTimeout(updateWorkflowFromCanvas, 100)
    },
    [setNodes, updateWorkflowFromCanvas, reactFlowInstance, nodes, toast]
  )

  // Initialize drag and drop handling
  const { dragState, handlers } = useWorkflowDnd({
    onDrop: createNodeFromDrop,
    enabled: !readonly
  })

  // Show drop zone indicator
  const showDropZone = useDropZoneIndicator(dragState.isOverDropZone)

  // Helper function to find optimal position for new nodes
  const findOptimalNodePosition = (
    targetPosition: { x: number; y: number },
    existingPositions: { x: number; y: number }[]
  ) => {
    const gridSize = 20
    const nodeSpacing = 100

    // Snap to grid
    let x = Math.round(targetPosition.x / gridSize) * gridSize
    let y = Math.round(targetPosition.y / gridSize) * gridSize

    // Check for overlapping nodes and adjust position
    const isOverlapping = (pos: { x: number; y: number }) => {
      return existingPositions.some(
        (existing) =>
          Math.abs(existing.x - pos.x) < nodeSpacing &&
          Math.abs(existing.y - pos.y) < nodeSpacing
      )
    }

    // Find non-overlapping position
    while (isOverlapping({ x, y })) {
      x += nodeSpacing
      if (x > targetPosition.x + nodeSpacing * 3) {
        x = targetPosition.x
        y += nodeSpacing
      }
    }

    return { x, y }
  }

  const updateNodeConfig = useCallback(
    (nodeId: string, config: Partial<WorkflowTask>) => {
      setNodes((nds) =>
        nds.map((node) => {
          if (node.id === nodeId) {
            return {
              ...node,
              data: {
                ...node.data,
                config: { ...node.data.config, ...config }
              }
            }
          }
          return node
        })
      )
      // Update workflow after node config change
      setTimeout(updateWorkflowFromCanvas, 100)
    },
    [setNodes, updateWorkflowFromCanvas]
  )

  const deleteNode = useCallback(
    (nodeId: string) => {
      setNodes((nds) => nds.filter((node) => node.id !== nodeId))
      setEdges((eds) =>
        eds.filter((edge) => edge.source !== nodeId && edge.target !== nodeId)
      )
      setSelectedNode(null)
      // Update workflow after deleting node
      setTimeout(updateWorkflowFromCanvas, 100)
    },
    [setNodes, setEdges, updateWorkflowFromCanvas]
  )

  // Memoize ReactFlow props for performance
  const reactFlowProps = useMemo(
    () => ({
      nodes,
      edges,
      onNodesChange,
      onEdgesChange,
      onConnect,
      onNodeClick,
      onPaneClick,
      onDrop: handlers.onDrop,
      onDragOver: handlers.onDragOver,
      onDragLeave: handlers.onDragLeave,
      nodeTypes,
      fitView: true,
      attributionPosition: 'bottom-left' as const,
      // Touch-friendly settings
      zoomOnScroll: !isMobile,
      zoomOnPinch: true,
      panOnScroll: isMobile,
      panOnDrag: true,
      preventScrolling: true,
      nodesDraggable: !readonly,
      nodesConnectable: !readonly,
      elementsSelectable: !readonly,
      className: cn(
        'bg-background',
        showDropZone &&
          'ring-2 ring-primary ring-offset-2 transition-all duration-200'
      )
    }),
    [
      nodes,
      edges,
      onNodesChange,
      onEdgesChange,
      onConnect,
      onNodeClick,
      onPaneClick,
      handlers,
      nodeTypes,
      isMobile,
      readonly,
      showDropZone
    ]
  )

  return (
    <div className="flex h-screen overflow-hidden">
      {/* Desktop Task Palette - Left Sidebar */}
      {!readonly && !isTablet && (
        <div className="w-80 overflow-hidden border-r bg-muted/30">
          <TaskPalette onTaskDrop={createNodeFromDrop} />
        </div>
      )}

      {/* Mobile/Tablet Task Palette - Sheet */}
      {!readonly && isTablet && (
        <Sheet open={showTaskPalette} onOpenChange={setShowTaskPalette}>
          <SheetContent side="left" className="w-[300px] p-0 sm:w-[400px]">
            <TaskPalette onTaskDrop={createNodeFromDrop} />
          </SheetContent>
        </Sheet>
      )}

      {/* Main Canvas Area */}
      <div className="relative flex-1 overflow-hidden">
        {/* Canvas Controls */}
        <div className="absolute left-4 top-4 z-10 flex items-start gap-2">
          {/* Mobile menu button */}
          {!readonly && isTablet && (
            <Button
              size="icon"
              variant="outline"
              onClick={() => setShowTaskPalette(!showTaskPalette)}
              className="bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60"
            >
              <Menu className="h-4 w-4" />
            </Button>
          )}

          <CanvasControls
            onMetadataClick={() => setShowMetadataEditor(!showMetadataEditor)}
            onValidate={() => {
              setShowValidationDialog(true)
            }}
            onSave={() => {
              if (workflow && onWorkflowChange) {
                // Trigger save through parent component
                updateWorkflowFromCanvas()
                setHasUnsavedChanges(false)
                toast({
                  title: 'Workflow saved',
                  description: 'Your changes have been saved successfully'
                })
              }
            }}
            onImport={() => setShowImportDialog(true)}
            onExport={() => setShowExportDialog(true)}
            readonly={readonly}
            hasUnsavedChanges={hasUnsavedChanges}
          />
        </div>

        {/* Metadata Editor - Responsive positioning */}
        {showMetadataEditor && !readonly && (
          <div
            className={`absolute z-10 ${isMobile ? 'inset-4' : 'left-4 top-16 w-80'}`}
          >
            <WorkflowMetadataEditor
              workflow={workflow}
              onWorkflowChange={onWorkflowChange}
              onClose={() => setShowMetadataEditor(false)}
            />
          </div>
        )}

        {/* React Flow Canvas */}
        <div className="relative h-full w-full">
          {/* Drop zone overlay indicator */}
          {showDropZone && dragState.isDragging && (
            <div className="pointer-events-none absolute inset-0 z-20">
              <div className="flex h-full w-full items-center justify-center">
                <div className="rounded-lg bg-primary/10 p-8 backdrop-blur-sm">
                  <p className="text-lg font-medium text-primary">
                    Drop here to add {dragState.draggedTaskType} task
                  </p>
                </div>
              </div>
            </div>
          )}

          <ReactFlow
            {...reactFlowProps}
            onInit={(instance) => setReactFlowInstance(instance)}
          >
            <Controls
              showZoom={!isMobile}
              showFitView
              showInteractive={false}
              position="bottom-right"
            />
            {!isMobile && <MiniMap />}
            <Background
              variant={BackgroundVariant.Dots}
              gap={isMobile ? 16 : 12}
              size={1}
            />
          </ReactFlow>
        </div>
      </div>

      {/* Desktop Task Configuration Panel - Right Sidebar */}
      {selectedNode && !readonly && !isTablet && (
        <div className="w-96 overflow-hidden border-l bg-muted/30">
          <TaskConfigurationPanel
            node={selectedNode}
            onUpdateConfig={(config) =>
              updateNodeConfig(selectedNode.id, config)
            }
            onDelete={() => deleteNode(selectedNode.id)}
            onClose={() => setSelectedNode(null)}
          />
        </div>
      )}

      {/* Mobile/Tablet Task Configuration Panel - Sheet */}
      {selectedNode && !readonly && isTablet && (
        <Sheet
          open={!!selectedNode}
          onOpenChange={(open) => !open && setSelectedNode(null)}
        >
          <SheetContent side="right" className="w-[300px] p-0 sm:w-[400px]">
            <TaskConfigurationPanel
              node={selectedNode}
              onUpdateConfig={(config) =>
                updateNodeConfig(selectedNode.id, config)
              }
              onDelete={() => deleteNode(selectedNode.id)}
              onClose={() => setSelectedNode(null)}
            />
          </SheetContent>
        </Sheet>
      )}

      {/* Validation Dialog */}
      {workflow && (
        <WorkflowValidator
          workflow={workflow}
          open={showValidationDialog}
          onOpenChange={setShowValidationDialog}
          onTestExecute={(testData) => {
            console.log('Test execution data:', testData)
            // In a real implementation, this would trigger a test execution
          }}
        />
      )}

      {/* Import Dialog */}
      <WorkflowImportExport
        open={showImportDialog}
        onOpenChange={setShowImportDialog}
        mode="import"
        onImport={(importedWorkflow) => {
          // Update the canvas with imported workflow
          if (onWorkflowChange) {
            onWorkflowChange(importedWorkflow)
            setIsInitialized(false) // Force re-initialization
            toast({
              title: 'Workflow imported',
              description: 'The workflow has been imported successfully'
            })
          }
        }}
      />

      {/* Export Dialog */}
      <WorkflowImportExport
        workflow={workflow}
        open={showExportDialog}
        onOpenChange={setShowExportDialog}
        mode="export"
      />
    </div>
  )
}

// Export with ReactFlowProvider for touch handling
export function WorkflowCanvas(props: WorkflowCanvasProps) {
  return (
    <ReactFlowProvider>
      <WorkflowCanvasInner {...props} />
    </ReactFlowProvider>
  )
}
