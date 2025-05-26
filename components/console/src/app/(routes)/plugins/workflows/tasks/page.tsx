'use client'

import React, { useState } from 'react'
import {
  Search,
  Plus,
  Filter,
  Grid3X3,
  List,
  Globe,
  GitBranch,
  Square,
  Layers,
  Puzzle,
  Zap,
  Settings,
  TestTube,
  Eye,
  Edit
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'

// Mock data for task types
const taskTypes = [
  {
    id: 'HTTP',
    name: 'HTTP Task',
    description: 'Make HTTP requests to external services and APIs',
    icon: Globe,
    category: 'Integration',
    color: 'blue',
    usageCount: 245,
    isBuiltIn: true,
    examples: ['API calls', 'Webhook triggers', 'REST integrations']
  },
  {
    id: 'SWITCH',
    name: 'Switch Task',
    description: 'Conditional logic and decision-making based on input values',
    icon: GitBranch,
    category: 'Logic',
    color: 'purple',
    usageCount: 189,
    isBuiltIn: true,
    examples: ['Conditional routing', 'Business logic', 'Data validation']
  },
  {
    id: 'TERMINATE',
    name: 'Terminate Task',
    description: 'End workflow execution with specific status and output',
    icon: Square,
    category: 'Control',
    color: 'red',
    usageCount: 156,
    isBuiltIn: true,
    examples: ['Early termination', 'Error handling', 'Success completion']
  },
  {
    id: 'SUB_WORKFLOW',
    name: 'Sub-workflow',
    description: 'Execute another workflow as a nested task',
    icon: Layers,
    category: 'Orchestration',
    color: 'green',
    usageCount: 98,
    isBuiltIn: true,
    examples: ['Reusable workflows', 'Modular design', 'Complex orchestration']
  },
  {
    id: 'DYNAMIC',
    name: 'Dynamic Task',
    description: 'Execute tasks dynamically based on input parameters',
    icon: Zap,
    category: 'Advanced',
    color: 'orange',
    usageCount: 67,
    isBuiltIn: true,
    examples: ['Dynamic routing', 'Runtime decisions', 'Flexible execution']
  },
  {
    id: 'CUSTOM_PAYMENT',
    name: 'Payment Processor',
    description: 'Custom task for processing payments with fees and validation',
    icon: Puzzle,
    category: 'Custom',
    color: 'indigo',
    usageCount: 42,
    isBuiltIn: false,
    examples: [
      'Payment processing',
      'Fee calculation',
      'Transaction validation'
    ]
  },
  {
    id: 'CUSTOM_KYC',
    name: 'KYC Validator',
    description: 'Custom task for Know Your Customer verification processes',
    icon: Puzzle,
    category: 'Custom',
    color: 'pink',
    usageCount: 28,
    isBuiltIn: false,
    examples: [
      'Identity verification',
      'Document validation',
      'Compliance checks'
    ]
  }
]

const categories = [
  'All',
  'Integration',
  'Logic',
  'Control',
  'Orchestration',
  'Advanced',
  'Custom'
]

export default function TasksPage() {
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedCategory, setSelectedCategory] = useState('All')
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid')

  const filteredTasks = taskTypes.filter((task) => {
    const matchesSearch =
      task.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      task.description.toLowerCase().includes(searchQuery.toLowerCase())
    const matchesCategory =
      selectedCategory === 'All' || task.category === selectedCategory
    return matchesSearch && matchesCategory
  })

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

  return (
    <div className="space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Task Library</h1>
          <p className="text-muted-foreground">
            Manage task definitions, test implementations, and create custom
            task types
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Button variant="outline" size="sm">
            <TestTube className="mr-2 h-4 w-4" />
            Test Environment
          </Button>
          <Button>
            <Plus className="mr-2 h-4 w-4" />
            Create Custom Task
          </Button>
        </div>
      </div>

      {/* Filters and Search */}
      <div className="flex flex-wrap items-center gap-4">
        <div className="min-w-[300px] flex-1">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
            <Input
              placeholder="Search tasks by name or description..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-10"
            />
          </div>
        </div>

        <Select value={selectedCategory} onValueChange={setSelectedCategory}>
          <SelectTrigger className="w-[180px]">
            <Filter className="mr-2 h-4 w-4" />
            <SelectValue placeholder="Category" />
          </SelectTrigger>
          <SelectContent>
            {categories.map((category) => (
              <SelectItem key={category} value={category}>
                {category}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <div className="flex items-center rounded-lg border">
          <Button
            variant={viewMode === 'grid' ? 'default' : 'ghost'}
            size="sm"
            onClick={() => setViewMode('grid')}
            className="rounded-r-none"
          >
            <Grid3X3 className="h-4 w-4" />
          </Button>
          <Button
            variant={viewMode === 'list' ? 'default' : 'ghost'}
            size="sm"
            onClick={() => setViewMode('list')}
            className="rounded-l-none"
          >
            <List className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Task Statistics */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Total Tasks</p>
                <p className="text-2xl font-bold">{taskTypes.length}</p>
              </div>
              <Layers className="h-8 w-8 text-muted-foreground" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Built-in Tasks</p>
                <p className="text-2xl font-bold">
                  {taskTypes.filter((t) => t.isBuiltIn).length}
                </p>
              </div>
              <Settings className="h-8 w-8 text-muted-foreground" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Custom Tasks</p>
                <p className="text-2xl font-bold">
                  {taskTypes.filter((t) => !t.isBuiltIn).length}
                </p>
              </div>
              <Puzzle className="h-8 w-8 text-muted-foreground" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Total Usage</p>
                <p className="text-2xl font-bold">
                  {taskTypes.reduce((sum, t) => sum + t.usageCount, 0)}
                </p>
              </div>
              <Zap className="h-8 w-8 text-muted-foreground" />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Task Types Display */}
      <Tabs defaultValue="library" className="w-full">
        <TabsList>
          <TabsTrigger value="library">Task Library</TabsTrigger>
          <TabsTrigger value="testing">Testing Environment</TabsTrigger>
          <TabsTrigger value="documentation">Documentation</TabsTrigger>
        </TabsList>

        <TabsContent value="library" className="mt-6">
          {viewMode === 'grid' ? (
            <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
              {filteredTasks.map((task) => {
                const IconComponent = task.icon
                return (
                  <Card
                    key={task.id}
                    className="transition-shadow hover:shadow-md"
                  >
                    <CardHeader className="pb-3">
                      <div className="flex items-start justify-between">
                        <div className="flex items-center gap-3">
                          <div
                            className={`rounded-lg border p-2 ${getColorClasses(task.color)}`}
                          >
                            <IconComponent className="h-5 w-5" />
                          </div>
                          <div>
                            <CardTitle className="text-lg">
                              {task.name}
                            </CardTitle>
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
                          <Button variant="ghost" size="sm">
                            <Eye className="h-4 w-4" />
                          </Button>
                          <Button variant="ghost" size="sm">
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
                          <p className="mb-2 text-sm font-medium">
                            Common Examples
                          </p>
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
                          <Button variant="outline" size="sm">
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
          ) : (
            <div className="space-y-4">
              {filteredTasks.map((task) => {
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
                                {task.examples
                                  .slice(0, 3)
                                  .map((example, index) => (
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
                          <Button variant="outline" size="sm">
                            <TestTube className="mr-1 h-4 w-4" />
                            Test
                          </Button>
                          <Button variant="ghost" size="sm">
                            <Eye className="h-4 w-4" />
                          </Button>
                          <Button variant="ghost" size="sm">
                            <Edit className="h-4 w-4" />
                          </Button>
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                )
              })}
            </div>
          )}

          {filteredTasks.length === 0 && (
            <div className="py-12 text-center">
              <Puzzle className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
              <h3 className="mb-2 text-lg font-medium">No tasks found</h3>
              <p className="mb-4 text-muted-foreground">
                Try adjusting your search criteria or create a new custom task.
              </p>
              <Button>
                <Plus className="mr-2 h-4 w-4" />
                Create Custom Task
              </Button>
            </div>
          )}
        </TabsContent>

        <TabsContent value="testing" className="mt-6">
          <Card>
            <CardHeader>
              <CardTitle>Task Testing Environment</CardTitle>
              <CardDescription>
                Test individual tasks with sample inputs and validate their
                behavior before using in workflows.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="py-8 text-center">
                <TestTube className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
                <h3 className="mb-2 text-lg font-medium">
                  Task Testing Coming Soon
                </h3>
                <p className="text-muted-foreground">
                  Interactive task testing environment is under development.
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="documentation" className="mt-6">
          <Card>
            <CardHeader>
              <CardTitle>Task Documentation</CardTitle>
              <CardDescription>
                Comprehensive documentation for all task types, parameters, and
                examples.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="py-8 text-center">
                <Settings className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
                <h3 className="mb-2 text-lg font-medium">
                  Documentation Coming Soon
                </h3>
                <p className="text-muted-foreground">
                  Detailed task documentation and guides are being prepared.
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
