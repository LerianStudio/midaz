'use client'

import React from 'react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { TestTube, Eye, Edit } from 'lucide-react'

interface TaskType {
  id: string
  name: string
  description: string
  icon: React.ComponentType<{ className?: string }>
  category: string
  color: string
  usageCount: number
  isBuiltIn: boolean
  examples: string[]
}

interface TaskLibraryBrowserProps {
  tasks: TaskType[]
  viewMode: 'grid' | 'list'
  onTaskTest?: (taskId: string) => void
  onTaskView?: (taskId: string) => void
  onTaskEdit?: (taskId: string) => void
}

export function TaskLibraryBrowser({
  tasks,
  viewMode,
  onTaskTest,
  onTaskView,
  onTaskEdit
}: TaskLibraryBrowserProps) {
  const getColorClasses = (color: string) => {
    switch (color) {
      case 'blue':
        return 'bg-blue-100 text-blue-800 border-blue-200'
      case 'purple':
        return 'bg-purple-100 text-purple-800 border-purple-200'
      case 'red':
        return 'bg-red-100 text-red-800 border-red-200'
      case 'green':
        return 'bg-green-100 text-green-800 border-green-200'
      case 'orange':
        return 'bg-orange-100 text-orange-800 border-orange-200'
      case 'indigo':
        return 'bg-indigo-100 text-indigo-800 border-indigo-200'
      case 'pink':
        return 'bg-pink-100 text-pink-800 border-pink-200'
      default:
        return 'bg-gray-100 text-gray-800 border-gray-200'
    }
  }

  if (viewMode === 'grid') {
    return (
      <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
        {tasks.map((task) => {
          const IconComponent = task.icon
          return (
            <Card key={task.id} className="transition-shadow hover:shadow-md">
              <CardHeader className="pb-3">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`rounded-lg border p-2 ${getColorClasses(task.color)}`}
                    >
                      <IconComponent className="h-5 w-5" />
                    </div>
                    <div>
                      <CardTitle className="text-lg">{task.name}</CardTitle>
                      <div className="mt-1 flex items-center gap-2">
                        <Badge variant="secondary" className="text-xs">
                          {task.category}
                        </Badge>
                        {task.isBuiltIn && (
                          <Badge variant="outline" className="text-xs">
                            Built-in
                          </Badge>
                        )}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => onTaskView?.(task.id)}
                    >
                      <Eye className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => onTaskEdit?.(task.id)}
                    >
                      <Edit className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <CardDescription className="mb-4">
                  {task.description}
                </CardDescription>

                <div className="space-y-3">
                  <div>
                    <p className="mb-2 text-sm font-medium">Common Examples</p>
                    <div className="flex flex-wrap gap-1">
                      {task.examples.map((example, index) => (
                        <Badge
                          key={index}
                          variant="outline"
                          className="text-xs"
                        >
                          {example}
                        </Badge>
                      ))}
                    </div>
                  </div>

                  <div className="flex items-center justify-between text-sm text-muted-foreground">
                    <span>Used in {task.usageCount} workflows</span>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => onTaskTest?.(task.id)}
                    >
                      <TestTube className="mr-1 h-3 w-3" />
                      Test
                    </Button>
                  </div>
                </div>
              </CardContent>
            </Card>
          )
        })}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {tasks.map((task) => {
        const IconComponent = task.icon
        return (
          <Card key={task.id}>
            <CardContent className="p-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-4">
                  <div
                    className={`rounded-lg border p-2 ${getColorClasses(task.color)}`}
                  >
                    <IconComponent className="h-5 w-5" />
                  </div>
                  <div className="flex-1">
                    <div className="mb-1 flex items-center gap-3">
                      <h3 className="font-semibold">{task.name}</h3>
                      <Badge variant="secondary" className="text-xs">
                        {task.category}
                      </Badge>
                      {task.isBuiltIn && (
                        <Badge variant="outline" className="text-xs">
                          Built-in
                        </Badge>
                      )}
                    </div>
                    <p className="text-sm text-muted-foreground">
                      {task.description}
                    </p>
                    <div className="mt-2 flex items-center gap-4">
                      <span className="text-xs text-muted-foreground">
                        {task.usageCount} uses
                      </span>
                      <div className="flex gap-1">
                        {task.examples.slice(0, 3).map((example, index) => (
                          <Badge
                            key={index}
                            variant="outline"
                            className="text-xs"
                          >
                            {example}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => onTaskTest?.(task.id)}
                  >
                    <TestTube className="mr-1 h-4 w-4" />
                    Test
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => onTaskView?.(task.id)}
                  >
                    <Eye className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => onTaskEdit?.(task.id)}
                  >
                    <Edit className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
