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
import { WorkflowCanvasSkeleton } from '../loading-states'
import {
  ErrorHandlingWrapper,
  WorkflowError,
  WorkflowErrorType
} from '../error-handling-wrapper'
import {
  Workflow,
  WorkflowTask,
  TaskType
} from '@/core/domain/entities/workflow'
import { useMediaQuery } from '@/hooks/use-media-query'
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Menu, X, AlertCircle, Save, RefreshCw } from 'lucide-react'
import { useToast } from '@/hooks/use-toast'
import { useWorkflowDnd, useDropZoneIndicator } from '@/hooks/use-workflow-dnd'
import { cn } from '@/lib/utils'

const nodeTypes = {
  taskNode: TaskNodeComponent
}

interface WorkflowCanvasProps {
  workflow?: Workflow
  onWorkflowChange?: (workflow: Workflow) => void
  onSave?: (workflow: Workflow) => Promise<void>
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
  onSave,
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
  const [isSaving, setIsSaving] = useState(false)
  const [canvasError, setCanvasError] = useState<WorkflowError | null>(null)
  const [isLoading, setIsLoading] = useState(false)

  // Media queries for responsive design
  const isMobile = useMediaQuery('(max-width: 768px)')
  const isTablet = useMediaQuery('(max-width: 1024px)')

  // Convert nodes and edges to workflow structure
  const convertNodesToWorkflow = useCallback((): Workflow => {
    const tasks: WorkflowTask[] = nodes
      .filter((node) => node.type === 'taskNode')
      .map((node) => {
        const task = node.data.task as WorkflowTask
        const dependencies = edges
          .filter((edge) => edge.target === node.id)
          .map((edge) => edge.source)
          .filter((source) => source !== 'start')

        return {
          ...task,
          position: node.position,
          dependencies: dependencies.length > 0 ? dependencies : undefined
        }
      })

    return {
      ...workflow!,
      tasks
    }
  }, [nodes, edges, workflow])

  // Auto-save functionality with error handling
  const autoSave = useCallback(async () => {
    if (!hasUnsavedChanges || !onSave || readonly) return

    try {
      setIsSaving(true)
      const currentWorkflow = convertNodesToWorkflow()
      await onSave(currentWorkflow)
      setHasUnsavedChanges(false)
      toast({
        title: 'Auto-saved',
        description: 'Your changes have been saved automatically'
      })
    } catch (error) {
      const workflowError = WorkflowError.fromError(error)
      setCanvasError(workflowError)
      toast({
        title: 'Auto-save failed',
        description: workflowError.message,
        variant: 'destructive'
      })
    } finally {
      setIsSaving(false)
    }
  }, [hasUnsavedChanges, onSave, readonly, convertNodesToWorkflow, toast])

  // Define callbacks before using them
  const handleTaskUpdate = useCallback(
    (taskId: string, updatedTask: WorkflowTask) => {
      if (readonly) return

      try {
        setNodes((nds) =>
          nds.map((node) => {
            if (node.id === taskId) {
              return {
                ...node,
                data: {
                  ...node.data,
                  task: updatedTask
                }
              }
            }
            return node
          })
        )
        setHasUnsavedChanges(true)
      } catch (error) {
        toast({
          title: 'Update failed',
          description: 'Failed to update task',
          variant: 'destructive'
        })
      }
    },
    [readonly, setNodes, toast]
  )

  const handleTaskDelete = useCallback(
    (taskId: string) => {
      if (readonly) return

      try {
        setNodes((nds) => nds.filter((node) => node.id !== taskId))
        setEdges((eds) =>
          eds.filter((edge) => edge.source !== taskId && edge.target !== taskId)
        )
        setHasUnsavedChanges(true)

        if (selectedNode?.id === taskId) {
          setSelectedNode(null)
        }
      } catch (error) {
        toast({
          title: 'Delete failed',
          description: 'Failed to delete task',
          variant: 'destructive'
        })
      }
    },
    [readonly, selectedNode, setNodes, setEdges, toast]
  )

  // Convert and trigger workflow change when nodes/edges change
  useEffect(() => {
    if (hasUnsavedChanges && onWorkflowChange && isInitialized) {
      const updatedWorkflow = convertNodesToWorkflow()
      onWorkflowChange(updatedWorkflow)
    }
  }, [
    nodes,
    edges,
    hasUnsavedChanges,
    onWorkflowChange,
    isInitialized,
    convertNodesToWorkflow
  ])

  // Auto-save every 30 seconds
  useEffect(() => {
    const interval = setInterval(autoSave, 30000)
    return () => clearInterval(interval)
  }, [autoSave])

  // Convert workflow tasks to nodes and edges when workflow changes
  useEffect(() => {
    if (
      !workflow ||
      !workflow.tasks ||
      workflow.tasks.length === 0 ||
      isInitialized
    )
      return

    setIsLoading(true)
    try {
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
      const taskNodeMap = new Map<string, Node>()

      // Create nodes for each task
      workflow.tasks.forEach((task, index) => {
        const taskId = task.taskReferenceName || `task-${index}`
        const node: Node = {
          id: taskId,
          type: 'taskNode',
          data: {
            task,
            onUpdate: (updatedTask: WorkflowTask) =>
              handleTaskUpdate(taskId, updatedTask),
            onDelete: () => handleTaskDelete(taskId)
          },
          position: (task as any).position || {
            x: 100 + (index + 1) * 200,
            y: 100
          }
        }
        newNodes.push(node)
        taskNodeMap.set(taskId, node)
      })

      // Create edges based on task order (connect each task to start node for now)
      // In a real implementation, edges would be based on the workflow definition
      workflow.tasks.forEach((task, index) => {
        const taskId = task.taskReferenceName || `task-${index}`
        
        // For now, connect all tasks to the start node
        // You would typically derive connections from the workflow structure
        const edge: Edge = {
          id: `start-${taskId}`,
          source: 'start',
          target: taskId,
          animated: true
        }
        newEdges.push(edge)
      })

      // Add end node
      const endNode: Node = {
        id: 'end',
        type: 'output',
        data: { label: 'End' },
        position: { x: 100 + (workflow.tasks.length + 1) * 200, y: 100 },
        style: {
          background: '#ef4444',
          color: 'white',
          border: '1px solid #dc2626',
          borderRadius: '8px'
        }
      }
      newNodes.push(endNode)

      setNodes(newNodes)
      setEdges(newEdges)
      setIsInitialized(true)
    } catch (error) {
      const workflowError = WorkflowError.fromError(error)
      setCanvasError(workflowError)
      toast({
        title: 'Failed to load workflow',
        description: workflowError.message,
        variant: 'destructive'
      })
    } finally {
      setIsLoading(false)
    }
  }, [workflow, isInitialized, handleTaskUpdate, handleTaskDelete, toast])

  // Error recovery
  const handleErrorRecovery = useCallback(() => {
    setCanvasError(null)
    setIsInitialized(false)
    // Trigger re-initialization
    if (workflow) {
      setIsLoading(true)
      setTimeout(() => {
        setIsLoading(false)
      }, 100)
    }
  }, [workflow])

  const onConnect = useCallback(
    (params: Connection) => {
      if (readonly) return

      try {
        setEdges((eds) => addEdge({ ...params, animated: true }, eds))
        setHasUnsavedChanges(true)
      } catch (error) {
        toast({
          title: 'Connection failed',
          description: 'Failed to create connection between tasks',
          variant: 'destructive'
        })
      }
    },
    [readonly, setEdges]
  )

  const onNodeClick = useCallback(
    (event: React.MouseEvent, node: Node) => {
      if (node.id !== 'start' && node.id !== 'end') {
        setSelectedNode(node)
        if (isMobile) {
          setShowConfigPanel(true)
        }
      }
    },
    [isMobile]
  )

  const handleManualSave = async () => {
    if (!onSave || readonly) return

    try {
      setIsSaving(true)
      const currentWorkflow = convertNodesToWorkflow()
      await onSave(currentWorkflow)
      setHasUnsavedChanges(false)
      toast({
        title: 'Saved successfully',
        description: 'Your workflow has been saved'
      })
    } catch (error) {
      const workflowError = WorkflowError.fromError(error)
      toast({
        title: 'Save failed',
        description: workflowError.message,
        variant: 'destructive'
      })
    } finally {
      setIsSaving(false)
    }
  }

  // Show loading state
  if (isLoading) {
    return <WorkflowCanvasSkeleton />
  }

  // Show error state
  if (canvasError) {
    return (
      <ErrorHandlingWrapper error={canvasError} onRetry={handleErrorRecovery}>
        <></>
      </ErrorHandlingWrapper>
    )
  }

  return (
    <div className="relative h-full w-full">
      {/* Save indicator */}
      {hasUnsavedChanges && (
        <Alert className="absolute left-4 top-4 z-10 w-auto">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription className="flex items-center gap-2">
            You have unsaved changes
            <Button
              size="sm"
              variant="outline"
              onClick={handleManualSave}
              disabled={isSaving}
            >
              {isSaving ? (
                <>
                  <RefreshCw className="mr-2 h-3 w-3 animate-spin" />
                  Saving...
                </>
              ) : (
                <>
                  <Save className="mr-2 h-3 w-3" />
                  Save Now
                </>
              )}
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Rest of the canvas implementation... */}
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onNodeClick={onNodeClick}
        onInit={setReactFlowInstance}
        nodeTypes={nodeTypes}
        fitView
        className="bg-background"
      >
        <Background variant={BackgroundVariant.Dots} gap={12} size={1} />
        <Controls />
        <MiniMap />
      </ReactFlow>

      {/* Mobile-responsive panels */}
      {isMobile ? (
        <>
          <Sheet open={showTaskPalette} onOpenChange={setShowTaskPalette}>
            <SheetTrigger asChild>
              <Button
                className="absolute bottom-4 left-4 z-10"
                size="icon"
                variant="default"
              >
                <Menu className="h-4 w-4" />
              </Button>
            </SheetTrigger>
            <SheetContent side="left" className="w-80">
              <TaskPalette />
            </SheetContent>
          </Sheet>

          <Sheet open={showConfigPanel} onOpenChange={setShowConfigPanel}>
            <SheetContent side="right" className="w-80">
              {selectedNode && (
                <TaskConfigurationPanel
                  node={selectedNode}
                  onUpdateConfig={(updates: Partial<WorkflowTask>) =>
                    handleTaskUpdate(selectedNode.id, { ...selectedNode.data.task, ...updates })
                  }
                  onDelete={() => handleTaskDelete(selectedNode.id)}
                  onClose={() => setShowConfigPanel(false)}
                />
              )}
            </SheetContent>
          </Sheet>
        </>
      ) : (
        <>
          <div className="absolute left-0 top-0 h-full w-64 border-r bg-background p-4">
            <TaskPalette />
          </div>
          {selectedNode && (
            <div className="absolute right-0 top-0 h-full w-80 border-l bg-background p-4">
              <TaskConfigurationPanel
                node={selectedNode}
                onUpdateConfig={(updates: Partial<WorkflowTask>) =>
                  handleTaskUpdate(selectedNode.id, { ...selectedNode.data.task, ...updates })
                }
                onDelete={() => handleTaskDelete(selectedNode.id)}
                onClose={() => setSelectedNode(null)}
              />
            </div>
          )}
        </>
      )}
    </div>
  )
}

export function WorkflowCanvasEnhanced(props: WorkflowCanvasProps) {
  return (
    <ReactFlowProvider>
      <WorkflowCanvasInner {...props} />
    </ReactFlowProvider>
  )
}
