'use client'

import React from 'react'
import { useState, useCallback, useMemo } from 'react'
import {
  ReactFlow,
  Node,
  Edge,
  addEdge,
  useNodesState,
  useEdgesState,
  Connection,
  Controls,
  Background,
  MiniMap,
  Handle,
  Position,
  NodeProps
} from 'reactflow'
import 'reactflow/dist/style.css'

import { DollarSign, Building, Plus, Settings, Trash2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle
} from '@/components/ui/sheet'

import {
  type TransactionRoute,
  type OperationRoute,
  mockAccountTypes
} from '@/components/accounting/mock/transaction-route-mock-data'

interface OperationNodeData {
  operation: OperationRoute
  onEdit: (operation: OperationRoute) => void
  onDelete: (operationId: string) => void
}

interface TransactionRouteDesignerProps {
  route: TransactionRoute
  onChange: (route: TransactionRoute) => void
  mode?: 'edit' | 'view'
}

// Custom node component for operations
function OperationNode({ data }: NodeProps<OperationNodeData>) {
  const { operation, onEdit, onDelete } = data

  const sourceAccountType = mockAccountTypes.find(
    (at) => at.id === operation.sourceAccountTypeId
  )
  const destinationAccountType = mockAccountTypes.find(
    (at) => at.id === operation.destinationAccountTypeId
  )

  const isDebit = operation.operationType === 'debit'

  return (
    <div
      className={`min-w-[280px] rounded-lg border-2 bg-white p-4 shadow-lg ${
        isDebit ? 'border-red-200 bg-red-50' : 'border-green-200 bg-green-50'
      }`}
    >
      <Handle type="target" position={Position.Top} />

      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <Badge
            variant={isDebit ? 'destructive' : 'secondary'}
            className="text-xs"
          >
            {operation.operationType.toUpperCase()}
          </Badge>
          <div className="flex items-center space-x-1">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onEdit(operation)}
              className="h-6 w-6 p-0"
            >
              <Settings className="h-3 w-3" />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onDelete(operation.id)}
              className="h-6 w-6 p-0 text-red-600 hover:text-red-700"
            >
              <Trash2 className="h-3 w-3" />
            </Button>
          </div>
        </div>

        <div className="text-sm font-medium">{operation.description}</div>

        <div className="space-y-2 text-xs">
          <div className="flex items-center space-x-2">
            <Building className="h-3 w-3" />
            <span className="text-muted-foreground">From:</span>
            <span>{sourceAccountType?.name || 'Unknown'}</span>
          </div>
          <div className="flex items-center space-x-2">
            <Building className="h-3 w-3" />
            <span className="text-muted-foreground">To:</span>
            <span>{destinationAccountType?.name || 'Unknown'}</span>
          </div>
          <div className="flex items-center space-x-2">
            <DollarSign className="h-3 w-3" />
            <span className="text-muted-foreground">Amount:</span>
            <span className="font-mono">{operation.amount.expression}</span>
          </div>
        </div>

        {operation.conditions && operation.conditions.length > 0 && (
          <div className="border-t pt-2">
            <div className="text-xs text-muted-foreground">
              Conditions: {operation.conditions.length}
            </div>
          </div>
        )}

        <div className="text-right text-xs text-muted-foreground">
          Step {operation.order}
        </div>
      </div>

      <Handle type="source" position={Position.Bottom} />
    </div>
  )
}

// Node types
const nodeTypes = {
  operation: OperationNode
}

export function TransactionRouteDesigner({
  route,
  onChange,
  mode = 'edit'
}: TransactionRouteDesignerProps) {
  const [selectedOperation, setSelectedOperation] =
    useState<OperationRoute | null>(null)
  const [isEditing, setIsEditing] = useState(false)
  const [sampleData, setSampleData] = useState({ amount: 100, currency: 'USD' })

  // Convert operations to nodes
  const initialNodes: Node[] = useMemo(() => {
    return route.operationRoutes.map((operation, index) => ({
      id: operation.id,
      type: 'operation',
      position: {
        x: 50 + (index % 3) * 320,
        y: 50 + Math.floor(index / 3) * 200
      },
      data: {
        operation,
        onEdit: (op: OperationRoute) => {
          setSelectedOperation(op)
          setIsEditing(true)
        },
        onDelete: (operationId: string) => {
          const updatedRoute = {
            ...route,
            operationRoutes: route.operationRoutes.filter(
              (op) => op.id !== operationId
            )
          }
          onChange(updatedRoute)
        }
      }
    }))
  }, [route.operationRoutes, onChange])

  // Convert operation flow to edges
  const initialEdges: Edge[] = useMemo(() => {
    const sortedOperations = [...route.operationRoutes].sort(
      (a, b) => a.order - b.order
    )
    return sortedOperations.slice(0, -1).map((operation, index) => ({
      id: `edge-${operation.id}-${sortedOperations[index + 1].id}`,
      source: operation.id,
      target: sortedOperations[index + 1].id,
      type: 'smoothstep',
      animated: true,
      style: { stroke: '#6366f1', strokeWidth: 2 }
    }))
  }, [route.operationRoutes])

  const [nodes, _setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)

  const onConnect = useCallback(
    (params: Connection) => setEdges((eds) => addEdge(params, eds)),
    [setEdges]
  )

  const handleAddOperation = () => {
    const newOperation: OperationRoute = {
      id: `op-${Date.now()}`,
      transactionRouteId: route.id,
      operationType: 'debit',
      sourceAccountTypeId: mockAccountTypes[0].id,
      destinationAccountTypeId: mockAccountTypes[1].id,
      amount: {
        expression: '{{amount}}',
        description: 'Transaction amount'
      },
      description: 'New operation',
      order: route.operationRoutes.length + 1
    }

    setSelectedOperation(newOperation)
    setIsEditing(true)
  }

  const handleSaveOperation = (operation: OperationRoute) => {
    const isNew = !route.operationRoutes.find((op) => op.id === operation.id)

    let updatedOperations
    if (isNew) {
      updatedOperations = [...route.operationRoutes, operation]
    } else {
      updatedOperations = route.operationRoutes.map((op) =>
        op.id === operation.id ? operation : op
      )
    }

    const updatedRoute = {
      ...route,
      operationRoutes: updatedOperations
    }

    onChange(updatedRoute)
    setIsEditing(false)
    setSelectedOperation(null)
  }

  const calculatePreview = (operation: OperationRoute) => {
    try {
      // Simple expression evaluation for preview
      const expression = operation.amount.expression
        .replace(/\{\{amount\}\}/g, sampleData.amount.toString())
        .replace(/\{\{(\w+)\}\}/g, (match, _field) => {
          // Handle other template variables
          return match // Keep as-is if we don't have the value
        })

      // Basic math evaluation (this is simplified - in production use a proper expression parser)
      const result = eval(expression)
      return isNaN(result) ? expression : result.toFixed(2)
    } catch {
      return operation.amount.expression
    }
  }

  return (
    <div className="h-[600px] w-full overflow-hidden rounded-lg border">
      <div className="flex h-full">
        {/* Main canvas */}
        <div className="relative flex-1">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            nodeTypes={nodeTypes}
            fitView
            className="bg-gray-50"
          >
            <Background />
            <Controls />
            <MiniMap />
          </ReactFlow>

          {mode === 'edit' && (
            <div className="absolute right-4 top-4 space-y-2">
              <Button onClick={handleAddOperation} className="gap-2 shadow-lg">
                <Plus className="h-4 w-4" />
                Add Operation
              </Button>
            </div>
          )}
        </div>

        {/* Side panel */}
        <div className="w-80 overflow-y-auto border-l bg-white">
          <div className="space-y-4 p-4">
            <div>
              <h3 className="mb-2 font-medium">Route Preview</h3>
              <div className="space-y-1 text-sm text-muted-foreground">
                <p>
                  <strong>Operations:</strong> {route.operationRoutes.length}
                </p>
                <p>
                  <strong>Status:</strong> {route.status}
                </p>
                <p>
                  <strong>Version:</strong> {route.version}
                </p>
              </div>
            </div>

            <div>
              <h3 className="mb-2 font-medium">Test Data</h3>
              <div className="space-y-2">
                <div>
                  <Label htmlFor="amount">Amount</Label>
                  <Input
                    id="amount"
                    type="number"
                    value={sampleData.amount}
                    onChange={(e) =>
                      setSampleData((prev) => ({
                        ...prev,
                        amount: parseFloat(e.target.value) || 0
                      }))
                    }
                  />
                </div>
                <div>
                  <Label htmlFor="currency">Currency</Label>
                  <Input
                    id="currency"
                    value={sampleData.currency}
                    onChange={(e) =>
                      setSampleData((prev) => ({
                        ...prev,
                        currency: e.target.value
                      }))
                    }
                  />
                </div>
              </div>
            </div>

            <div>
              <h3 className="mb-2 font-medium">Operation Flow</h3>
              <div className="space-y-2">
                {route.operationRoutes
                  .sort((a, b) => a.order - b.order)
                  .map((operation) => (
                    <Card key={operation.id} className="p-2">
                      <div className="space-y-1">
                        <div className="flex items-center justify-between">
                          <Badge
                            variant={
                              operation.operationType === 'debit'
                                ? 'destructive'
                                : 'secondary'
                            }
                            className="text-xs"
                          >
                            {operation.operationType}
                          </Badge>
                          <span className="text-xs text-muted-foreground">
                            Step {operation.order}
                          </span>
                        </div>
                        <div className="text-xs font-medium">
                          {operation.description}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          Preview: {sampleData.currency}{' '}
                          {calculatePreview(operation)}
                        </div>
                      </div>
                    </Card>
                  ))}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Operation Editor Sheet */}
      <Sheet open={isEditing} onOpenChange={setIsEditing}>
        <SheetContent className="w-[500px] sm:w-[500px]">
          <SheetHeader>
            <SheetTitle>
              {selectedOperation &&
              route.operationRoutes.find((op) => op.id === selectedOperation.id)
                ? 'Edit Operation'
                : 'Add Operation'}
            </SheetTitle>
            <SheetDescription>
              Configure the operation details and account mappings.
            </SheetDescription>
          </SheetHeader>

          {selectedOperation && (
            <OperationEditor
              operation={selectedOperation}
              onSave={handleSaveOperation}
              onCancel={() => {
                setIsEditing(false)
                setSelectedOperation(null)
              }}
            />
          )}
        </SheetContent>
      </Sheet>
    </div>
  )
}

// Operation Editor Component
interface OperationEditorProps {
  operation: OperationRoute
  onSave: (operation: OperationRoute) => void
  onCancel: () => void
}

function OperationEditor({
  operation,
  onSave,
  onCancel
}: OperationEditorProps) {
  const [formData, setFormData] = useState(operation)

  const handleSave = () => {
    onSave(formData)
  }

  return (
    <div className="space-y-4 pt-6">
      <div className="space-y-2">
        <Label htmlFor="description">Description</Label>
        <Input
          id="description"
          value={formData.description}
          onChange={(e) =>
            setFormData((prev) => ({ ...prev, description: e.target.value }))
          }
          placeholder="Describe this operation..."
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="operationType">Operation Type</Label>
        <Select
          value={formData.operationType}
          onValueChange={(value: 'debit' | 'credit') =>
            setFormData((prev) => ({ ...prev, operationType: value }))
          }
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="debit">Debit</SelectItem>
            <SelectItem value="credit">Credit</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="sourceAccount">Source Account Type</Label>
          <Select
            value={formData.sourceAccountTypeId}
            onValueChange={(value) =>
              setFormData((prev) => ({ ...prev, sourceAccountTypeId: value }))
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {mockAccountTypes.map((accountType) => (
                <SelectItem key={accountType.id} value={accountType.id}>
                  {accountType.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-2">
          <Label htmlFor="destinationAccount">Destination Account Type</Label>
          <Select
            value={formData.destinationAccountTypeId}
            onValueChange={(value) =>
              setFormData((prev) => ({
                ...prev,
                destinationAccountTypeId: value
              }))
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {mockAccountTypes.map((accountType) => (
                <SelectItem key={accountType.id} value={accountType.id}>
                  {accountType.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="amountExpression">Amount Expression</Label>
        <Input
          id="amountExpression"
          value={formData.amount.expression}
          onChange={(e) =>
            setFormData((prev) => ({
              ...prev,
              amount: { ...prev.amount, expression: e.target.value }
            }))
          }
          placeholder="e.g., {{amount}}, {{amount}} * 0.03"
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="amountDescription">Amount Description</Label>
        <Input
          id="amountDescription"
          value={formData.amount.description}
          onChange={(e) =>
            setFormData((prev) => ({
              ...prev,
              amount: { ...prev.amount, description: e.target.value }
            }))
          }
          placeholder="Describe what this amount represents..."
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="order">Order</Label>
        <Input
          id="order"
          type="number"
          value={formData.order}
          onChange={(e) =>
            setFormData((prev) => ({
              ...prev,
              order: parseInt(e.target.value) || 1
            }))
          }
        />
      </div>

      <div className="flex justify-end space-x-2 pt-6">
        <Button variant="outline" onClick={onCancel}>
          Cancel
        </Button>
        <Button onClick={handleSave}>Save Operation</Button>
      </div>
    </div>
  )
}
