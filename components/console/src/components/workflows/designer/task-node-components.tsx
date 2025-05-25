'use client'

import { memo } from 'react'
import { Handle, Position, NodeProps } from 'reactflow'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Globe,
  GitBranch,
  Square,
  Layers,
  Clock,
  User,
  StopCircle,
  Zap,
  MessageCircle,
  Settings,
  RotateCcw,
  Filter,
  MoreHorizontal,
  Edit,
  Trash2
} from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { TaskType } from '@/core/domain/entities/workflow'

interface TaskNodeData {
  label: string
  taskType: TaskType
  config: {
    name: string
    taskReferenceName: string
    type: TaskType
    inputParameters: Record<string, any>
    description?: string
    optional?: boolean
    retryCount?: number
    timeoutSeconds?: number
  }
}

const taskIcons = {
  HTTP: <Globe className="h-4 w-4" />,
  SWITCH: <GitBranch className="h-4 w-4" />,
  DECISION: <Filter className="h-4 w-4" />,
  FORK_JOIN: <Layers className="h-4 w-4" />,
  FORK_JOIN_DYNAMIC: <Layers className="h-4 w-4" />,
  JOIN: <Layers className="h-4 w-4" />,
  SUB_WORKFLOW: <Square className="h-4 w-4" />,
  EVENT: <MessageCircle className="h-4 w-4" />,
  WAIT: <Clock className="h-4 w-4" />,
  HUMAN: <User className="h-4 w-4" />,
  TERMINATE: <StopCircle className="h-4 w-4" />,
  LAMBDA: <Zap className="h-4 w-4" />,
  KAFKA_PUBLISH: <MessageCircle className="h-4 w-4" />,
  JSON_JQ_TRANSFORM: <Settings className="h-4 w-4" />,
  SET_VARIABLE: <RotateCcw className="h-4 w-4" />,
  CUSTOM: <Settings className="h-4 w-4" />
}

const taskColors = {
  HTTP: 'border-blue-300 bg-blue-50 dark:bg-blue-900/20',
  SWITCH: 'border-purple-300 bg-purple-50 dark:bg-purple-900/20',
  DECISION: 'border-indigo-300 bg-indigo-50 dark:bg-indigo-900/20',
  FORK_JOIN: 'border-green-300 bg-green-50 dark:bg-green-900/20',
  FORK_JOIN_DYNAMIC: 'border-green-300 bg-green-50 dark:bg-green-900/20',
  JOIN: 'border-green-300 bg-green-50 dark:bg-green-900/20',
  SUB_WORKFLOW: 'border-teal-300 bg-teal-50 dark:bg-teal-900/20',
  EVENT: 'border-cyan-300 bg-cyan-50 dark:bg-cyan-900/20',
  WAIT: 'border-orange-300 bg-orange-50 dark:bg-orange-900/20',
  HUMAN: 'border-pink-300 bg-pink-50 dark:bg-pink-900/20',
  TERMINATE: 'border-red-300 bg-red-50 dark:bg-red-900/20',
  LAMBDA: 'border-yellow-300 bg-yellow-50 dark:bg-yellow-900/20',
  KAFKA_PUBLISH: 'border-cyan-300 bg-cyan-50 dark:bg-cyan-900/20',
  JSON_JQ_TRANSFORM: 'border-emerald-300 bg-emerald-50 dark:bg-emerald-900/20',
  SET_VARIABLE: 'border-lime-300 bg-lime-50 dark:bg-lime-900/20',
  CUSTOM: 'border-gray-300 bg-gray-50 dark:bg-gray-900/20'
}

export const TaskNodeComponent = memo(
  ({ data, selected }: NodeProps<TaskNodeData>) => {
    const { taskType, config } = data
    const icon = taskIcons[taskType] || <Settings className="h-4 w-4" />
    const colorClass = taskColors[taskType] || taskColors.CUSTOM

    const getTaskDisplayName = () => {
      return config.name || config.taskReferenceName || taskType
    }

    const hasConfiguration = () => {
      return (
        Object.keys(config.inputParameters || {}).length > 0 ||
        config.description ||
        config.retryCount ||
        config.timeoutSeconds
      )
    }

    const isConditionalTask = () => {
      return ['SWITCH', 'DECISION'].includes(taskType)
    }

    const isParallelTask = () => {
      return ['FORK_JOIN', 'FORK_JOIN_DYNAMIC'].includes(taskType)
    }

    const handleEdit = (e: React.MouseEvent) => {
      e.stopPropagation()
      // This will be handled by the parent canvas component
      console.log('Edit task:', config.taskReferenceName)
    }

    const handleDelete = (e: React.MouseEvent) => {
      e.stopPropagation()
      // This will be handled by the parent canvas component
      console.log('Delete task:', config.taskReferenceName)
    }

    return (
      <>
        {/* Input Handle */}
        <Handle
          type="target"
          position={Position.Top}
          style={{
            background: '#555',
            width: 8,
            height: 8,
            border: '2px solid #fff'
          }}
        />

        <Card
          className={` ${colorClass} min-w-[200px] max-w-[280px] ${selected ? 'ring-2 ring-primary ring-offset-2' : ''} cursor-pointer transition-all duration-200 hover:shadow-md`}
        >
          <div className="p-3">
            {/* Header */}
            <div className="mb-2 flex items-start justify-between">
              <div className="flex min-w-0 flex-1 items-center space-x-2">
                {icon}
                <div className="min-w-0 flex-1">
                  <h4 className="truncate text-sm font-medium">
                    {getTaskDisplayName()}
                  </h4>
                  <p className="text-xs text-muted-foreground">{taskType}</p>
                </div>
              </div>

              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="sm" className="h-6 w-6 p-0">
                    <MoreHorizontal className="h-3 w-3" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={handleEdit}>
                    <Edit className="mr-2 h-3 w-3" />
                    Configure
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    onClick={handleDelete}
                    className="text-red-600"
                  >
                    <Trash2 className="mr-2 h-3 w-3" />
                    Delete
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>

            {/* Configuration Status */}
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-1">
                {hasConfiguration() && (
                  <Badge variant="secondary" className="text-xs">
                    Configured
                  </Badge>
                )}
                {config.optional && (
                  <Badge variant="outline" className="text-xs">
                    Optional
                  </Badge>
                )}
              </div>

              {(config.retryCount || config.timeoutSeconds) && (
                <div className="text-xs text-muted-foreground">
                  {config.retryCount && `${config.retryCount} retries`}
                  {config.retryCount && config.timeoutSeconds && ' • '}
                  {config.timeoutSeconds && `${config.timeoutSeconds}s timeout`}
                </div>
              )}
            </div>

            {/* Description */}
            {config.description && (
              <p className="mt-2 line-clamp-2 text-xs text-muted-foreground">
                {config.description}
              </p>
            )}
          </div>
        </Card>

        {/* Output Handles */}
        {isConditionalTask() ? (
          // Multiple output handles for conditional tasks
          <>
            <Handle
              type="source"
              position={Position.Bottom}
              id="true"
              style={{
                background: '#10b981',
                width: 8,
                height: 8,
                border: '2px solid #fff',
                left: '25%'
              }}
            />
            <Handle
              type="source"
              position={Position.Bottom}
              id="false"
              style={{
                background: '#ef4444',
                width: 8,
                height: 8,
                border: '2px solid #fff',
                left: '75%'
              }}
            />
          </>
        ) : isParallelTask() ? (
          // Multiple output handles for parallel tasks
          <>
            <Handle
              type="source"
              position={Position.Bottom}
              id="branch1"
              style={{
                background: '#6366f1',
                width: 8,
                height: 8,
                border: '2px solid #fff',
                left: '25%'
              }}
            />
            <Handle
              type="source"
              position={Position.Bottom}
              id="branch2"
              style={{
                background: '#6366f1',
                width: 8,
                height: 8,
                border: '2px solid #fff',
                left: '50%'
              }}
            />
            <Handle
              type="source"
              position={Position.Bottom}
              id="branch3"
              style={{
                background: '#6366f1',
                width: 8,
                height: 8,
                border: '2px solid #fff',
                left: '75%'
              }}
            />
          </>
        ) : (
          // Single output handle for most tasks
          <Handle
            type="source"
            position={Position.Bottom}
            style={{
              background: '#555',
              width: 8,
              height: 8,
              border: '2px solid #fff'
            }}
          />
        )}
      </>
    )
  }
)
