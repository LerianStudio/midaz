'use client'

import { useMemo, useState } from 'react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
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
  Search,
  GripVertical,
  Move
} from 'lucide-react'
import { TaskType } from '@/core/domain/entities/workflow'
import { useMediaQuery } from '@/hooks/use-media-query'
import { useWorkflowDnd } from '@/hooks/use-workflow-dnd'
import { cn } from '@/lib/utils'

interface TaskTypeInfo {
  type: TaskType
  name: string
  description: string
  icon: React.ReactNode
  category: string
  color: string
  examples: string[]
}

const taskTypes: TaskTypeInfo[] = [
  {
    type: 'HTTP',
    name: 'HTTP Request',
    description: 'Make HTTP calls to external services',
    icon: <Globe className="h-4 w-4" />,
    category: 'Integration',
    color: 'bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-200',
    examples: ['API calls', 'Webhooks', 'REST services']
  },
  {
    type: 'SWITCH',
    name: 'Switch Decision',
    description: 'Route workflow based on input values',
    icon: <GitBranch className="h-4 w-4" />,
    category: 'Control Flow',
    color:
      'bg-purple-100 text-purple-800 dark:bg-purple-800 dark:text-purple-200',
    examples: ['Conditional routing', 'Status checks', 'Multi-path logic']
  },
  {
    type: 'DECISION',
    name: 'Decision Task',
    description: 'Evaluate conditions and branch accordingly',
    icon: <Filter className="h-4 w-4" />,
    category: 'Control Flow',
    color:
      'bg-indigo-100 text-indigo-800 dark:bg-indigo-800 dark:text-indigo-200',
    examples: ['Boolean logic', 'Rule evaluation', 'Threshold checks']
  },
  {
    type: 'FORK_JOIN',
    name: 'Fork Join',
    description: 'Execute tasks in parallel and wait for completion',
    icon: <Layers className="h-4 w-4" />,
    category: 'Control Flow',
    color: 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200',
    examples: [
      'Parallel processing',
      'Concurrent API calls',
      'Batch operations'
    ]
  },
  {
    type: 'SUB_WORKFLOW',
    name: 'Sub Workflow',
    description: 'Execute another workflow as a task',
    icon: <Square className="h-4 w-4" />,
    category: 'Composition',
    color: 'bg-teal-100 text-teal-800 dark:bg-teal-800 dark:text-teal-200',
    examples: ['Workflow composition', 'Reusable processes', 'Modular design']
  },
  {
    type: 'WAIT',
    name: 'Wait Task',
    description: 'Pause workflow execution for a specified duration',
    icon: <Clock className="h-4 w-4" />,
    category: 'Timing',
    color:
      'bg-orange-100 text-orange-800 dark:bg-orange-800 dark:text-orange-200',
    examples: ['Delays', 'Cooling periods', 'Rate limiting']
  },
  {
    type: 'HUMAN',
    name: 'Human Task',
    description: 'Require human intervention or approval',
    icon: <User className="h-4 w-4" />,
    category: 'Approval',
    color: 'bg-pink-100 text-pink-800 dark:bg-pink-800 dark:text-pink-200',
    examples: ['Manual approval', 'Review tasks', 'Data entry']
  },
  {
    type: 'TERMINATE',
    name: 'Terminate',
    description: 'End workflow execution with a specific status',
    icon: <StopCircle className="h-4 w-4" />,
    category: 'Control Flow',
    color: 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200',
    examples: ['Early termination', 'Error handling', 'Completion states']
  },
  {
    type: 'LAMBDA',
    name: 'Lambda Function',
    description: 'Execute serverless functions',
    icon: <Zap className="h-4 w-4" />,
    category: 'Compute',
    color:
      'bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-200',
    examples: ['AWS Lambda', 'Function calls', 'Custom logic']
  },
  {
    type: 'EVENT',
    name: 'Event Task',
    description: 'Publish or wait for events',
    icon: <MessageCircle className="h-4 w-4" />,
    category: 'Messaging',
    color: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-800 dark:text-cyan-200',
    examples: ['Event publishing', 'Message queues', 'Notifications']
  },
  {
    type: 'JSON_JQ_TRANSFORM',
    name: 'JSON Transform',
    description: 'Transform JSON data using JQ expressions',
    icon: <Settings className="h-4 w-4" />,
    category: 'Data',
    color:
      'bg-emerald-100 text-emerald-800 dark:bg-emerald-800 dark:text-emerald-200',
    examples: ['Data transformation', 'JSON processing', 'Field mapping']
  },
  {
    type: 'SET_VARIABLE',
    name: 'Set Variable',
    description: 'Set workflow variables for later use',
    icon: <RotateCcw className="h-4 w-4" />,
    category: 'Data',
    color: 'bg-lime-100 text-lime-800 dark:bg-lime-800 dark:text-lime-200',
    examples: ['Variable assignment', 'State management', 'Data storage']
  }
]

const categories = Array.from(new Set(taskTypes.map((task) => task.category)))

interface TaskPaletteProps {
  onTaskDrop?: (taskType: TaskType, position: { x: number; y: number }) => void
}

export function TaskPalette({ onTaskDrop }: TaskPaletteProps) {
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedCategory, setSelectedCategory] = useState<string>('all')
  const [activeTaskType, setActiveTaskType] = useState<TaskType | null>(null)
  const isMobile = useMediaQuery('(max-width: 768px)')
  const isTablet = useMediaQuery('(max-width: 1024px)')

  const { dragState, handlers } = useWorkflowDnd({
    onDrop: onTaskDrop,
    enabled: true
  })

  // Filter tasks based on search query and category
  const filteredTasks = useMemo(() => {
    return taskTypes.filter((task) => {
      const matchesSearch =
        searchQuery === '' ||
        task.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        task.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
        task.examples.some((ex) =>
          ex.toLowerCase().includes(searchQuery.toLowerCase())
        )

      const matchesCategory =
        selectedCategory === 'all' || task.category === selectedCategory

      return matchesSearch && matchesCategory
    })
  }, [searchQuery, selectedCategory])

  const renderTaskCard = (task: TaskTypeInfo) => {
    const isDragging =
      dragState.isDragging && dragState.draggedTaskType === task.type
    const isActive = activeTaskType === task.type

    return (
      <Card
        key={task.type}
        className={cn(
          'transition-all duration-200',
          'cursor-grab hover:shadow-md',
          isDragging && 'scale-95 opacity-50',
          isActive && 'ring-2 ring-primary ring-offset-2',
          isMobile && 'touch-manipulation',
          'relative overflow-hidden'
        )}
        draggable={!isMobile}
        onDragStart={(event) => {
          if (!isMobile) {
            setActiveTaskType(task.type)
            handlers.onDragStart(event, task.type)
          }
        }}
        onDragEnd={() => {
          setActiveTaskType(null)
          handlers.onDragEnd()
        }}
        onTouchStart={(event) => {
          if (isMobile) {
            setActiveTaskType(task.type)
            handlers.onTouchStart(event, task.type)
          }
        }}
        onTouchMove={handlers.onTouchMove}
        onTouchEnd={(event) => {
          setActiveTaskType(null)
          handlers.onTouchEnd(event)
        }}
      >
        {/* Drag indicator overlay */}
        {isDragging && (
          <div className="absolute inset-0 bg-primary/10 backdrop-blur-sm" />
        )}

        <CardContent className={`${isMobile ? 'p-3' : 'p-4'}`}>
          <div className="flex items-start space-x-3">
            {!isMobile && (
              <div className="mt-0.5 text-muted-foreground">
                {isDragging ? (
                  <Move className="h-4 w-4 animate-pulse" />
                ) : (
                  <GripVertical className="h-4 w-4" />
                )}
              </div>
            )}
            <div className={`rounded-lg p-2 ${task.color} flex-shrink-0`}>
              {task.icon}
            </div>
            <div className="min-w-0 flex-1">
              <h4 className={`font-medium ${isMobile ? 'text-sm' : 'text-sm'}`}>
                {task.name}
              </h4>
              <p
                className={`mt-1 text-muted-foreground ${isMobile ? 'text-xs' : 'text-xs'}`}
              >
                {task.description}
              </p>
              {!isMobile && (
                <div className="mt-2 flex flex-wrap gap-1">
                  {task.examples.slice(0, 2).map((example) => (
                    <Badge key={example} variant="outline" className="text-xs">
                      {example}
                    </Badge>
                  ))}
                  {task.examples.length > 2 && (
                    <Badge variant="outline" className="text-xs">
                      +{task.examples.length - 2}
                    </Badge>
                  )}
                </div>
              )}
            </div>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className={`border-b ${isMobile ? 'p-3' : 'p-4'}`}>
        <h3 className="font-semibold">Task Palette</h3>
        <p
          className={`text-muted-foreground ${isMobile ? 'mt-1 text-xs' : 'text-sm'}`}
        >
          {isMobile
            ? 'Tap tasks to add to workflow'
            : 'Drag tasks to the canvas to build your workflow'}
        </p>

        {/* Search Input */}
        <div className="relative mt-3">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
          <Input
            placeholder="Search tasks..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="h-9 pl-9"
          />
        </div>
      </div>

      {/* Category Tabs for Mobile */}
      {isMobile && (
        <Tabs
          value={selectedCategory}
          onValueChange={setSelectedCategory}
          className="w-full"
        >
          <TabsList className="h-auto w-full flex-wrap justify-start px-2">
            <TabsTrigger value="all" className="text-xs">
              All
            </TabsTrigger>
            {categories.map((cat) => (
              <TabsTrigger key={cat} value={cat} className="text-xs">
                {cat}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      )}

      <ScrollArea className={`flex-1 ${isMobile ? 'p-3' : 'p-4'}`}>
        <div className={`${isMobile ? 'space-y-3' : 'space-y-6'}`}>
          {isMobile ? (
            // Mobile: Show filtered tasks without category grouping
            <div className="space-y-2">{filteredTasks.map(renderTaskCard)}</div>
          ) : (
            // Desktop: Show tasks grouped by category
            categories
              .filter(
                (category) =>
                  selectedCategory === 'all' || category === selectedCategory
              )
              .map((category) => {
                const categoryTasks = filteredTasks.filter(
                  (task) => task.category === category
                )
                if (categoryTasks.length === 0) return null

                return (
                  <div key={category}>
                    <h4 className="mb-3 flex items-center space-x-2 text-sm font-medium">
                      <span>{category}</span>
                      <Badge variant="secondary" className="text-xs">
                        {categoryTasks.length}
                      </Badge>
                    </h4>

                    <div className="space-y-2">
                      {categoryTasks.map(renderTaskCard)}
                    </div>
                  </div>
                )
              })
          )}
        </div>
      </ScrollArea>

      {!isMobile && (
        <div className="border-t bg-muted/30 p-4">
          <div className="text-xs text-muted-foreground">
            <p className="mb-1">
              💡 <strong>Tip:</strong> Drag any task to the canvas to add it to
              your workflow
            </p>
            <p>
              {dragState.isDragging
                ? '🎯 Drop the task on the canvas to create a new node'
                : 'Click on tasks in the canvas to configure their properties'}
            </p>
          </div>
        </div>
      )}
    </div>
  )
}
