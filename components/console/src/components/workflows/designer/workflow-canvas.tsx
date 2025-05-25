'use client'

import { useCallback, useState } from 'react'
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
  BackgroundVariant
} from 'reactflow'
import 'reactflow/dist/style.css'
import { TaskNodeComponent } from './task-node-components'
import { TaskPalette } from './task-palette'
import { TaskConfigurationPanel } from './task-configuration-panel'
import { WorkflowMetadataEditor } from './workflow-metadata-editor'
import { CanvasControls } from './canvas-controls'
import { Card } from '@/components/ui/card'
import { Workflow, WorkflowTask } from '@/core/domain/entities/workflow'

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

export function WorkflowCanvas({
  workflow,
  onWorkflowChange,
  readonly = false
}: WorkflowCanvasProps) {
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)
  const [selectedNode, setSelectedNode] = useState<Node | null>(null)
  const [showMetadataEditor, setShowMetadataEditor] = useState(false)

  const onConnect = useCallback(
    (params: Connection) => setEdges((eds) => addEdge(params, eds)),
    [setEdges]
  )

  const onNodeClick = useCallback((event: React.MouseEvent, node: Node) => {
    setSelectedNode(node)
  }, [])

  const onPaneClick = useCallback(() => {
    setSelectedNode(null)
  }, [])

  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }, [])

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault()

      const reactFlowBounds = event.currentTarget.getBoundingClientRect()
      const taskType = event.dataTransfer.getData('application/reactflow')

      if (typeof taskType === 'undefined' || !taskType) {
        return
      }

      const position = {
        x: event.clientX - reactFlowBounds.left,
        y: event.clientY - reactFlowBounds.top
      }

      const newNode: Node = {
        id: `${taskType.toLowerCase()}_${Date.now()}`,
        type: 'taskNode',
        position,
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
    },
    [setNodes]
  )

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
    },
    [setNodes]
  )

  const deleteNode = useCallback(
    (nodeId: string) => {
      setNodes((nds) => nds.filter((node) => node.id !== nodeId))
      setEdges((eds) =>
        eds.filter((edge) => edge.source !== nodeId && edge.target !== nodeId)
      )
      setSelectedNode(null)
    },
    [setNodes, setEdges]
  )

  return (
    <div className="flex h-screen">
      {/* Task Palette - Left Sidebar */}
      {!readonly && (
        <div className="w-80 border-r bg-muted/30">
          <TaskPalette />
        </div>
      )}

      {/* Main Canvas Area */}
      <div className="relative flex-1">
        {/* Canvas Controls */}
        <div className="absolute left-4 top-4 z-10">
          <CanvasControls
            onMetadataClick={() => setShowMetadataEditor(!showMetadataEditor)}
            onValidate={() => console.log('Validating workflow...')}
            onSave={() => console.log('Saving workflow...')}
            readonly={readonly}
          />
        </div>

        {/* Metadata Editor */}
        {showMetadataEditor && !readonly && (
          <div className="absolute left-4 top-16 z-10 w-80">
            <WorkflowMetadataEditor
              workflow={workflow}
              onWorkflowChange={onWorkflowChange}
              onClose={() => setShowMetadataEditor(false)}
            />
          </div>
        )}

        {/* React Flow Canvas */}
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onNodeClick={onNodeClick}
          onPaneClick={onPaneClick}
          onDrop={onDrop}
          onDragOver={onDragOver}
          nodeTypes={nodeTypes}
          fitView
          attributionPosition="bottom-left"
        >
          <Controls />
          <MiniMap />
          <Background variant={BackgroundVariant.Dots} gap={12} size={1} />
        </ReactFlow>
      </div>

      {/* Task Configuration Panel - Right Sidebar */}
      {selectedNode && !readonly && (
        <div className="w-96 border-l bg-muted/30">
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
    </div>
  )
}
