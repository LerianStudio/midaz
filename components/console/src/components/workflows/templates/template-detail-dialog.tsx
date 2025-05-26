'use client'

import { WorkflowTemplate } from '@/core/domain/entities/workflow-template'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import {
  Star,
  Download,
  Clock,
  Settings,
  Play,
  Users,
  Calendar,
  CheckCircle,
  AlertTriangle,
  Info,
  Code,
  BookOpen
} from 'lucide-react'

interface TemplateDetailDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  template: WorkflowTemplate
  onUseTemplate: () => void
}

export function TemplateDetailDialog({
  open,
  onOpenChange,
  template,
  onUseTemplate
}: TemplateDetailDialogProps) {
  const getComplexityColor = (complexity: string) => {
    switch (complexity) {
      case 'SIMPLE':
        return 'bg-green-100 text-green-800'
      case 'MEDIUM':
        return 'bg-yellow-100 text-yellow-800'
      case 'COMPLEX':
        return 'bg-orange-100 text-orange-800'
      case 'ADVANCED':
        return 'bg-red-100 text-red-800'
      default:
        return 'bg-gray-100 text-gray-800'
    }
  }

  const getTaskTypeIcon = (type: string) => {
    switch (type) {
      case 'HTTP_TASK':
        return '🌐'
      case 'DECISION':
        return '🔀'
      case 'SWITCH':
        return '🔄'
      case 'HUMAN_TASK':
        return '👤'
      case 'SUB_WORKFLOW':
        return '📋'
      case 'FORK_JOIN':
        return '🔀'
      default:
        return '⚙️'
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-4xl overflow-y-auto">
        <DialogHeader>
          <div className="flex items-start justify-between">
            <div className="flex-1">
              <DialogTitle className="text-xl">{template.name}</DialogTitle>
              <DialogDescription className="mt-1">
                {template.description}
              </DialogDescription>
            </div>
            <Button onClick={onUseTemplate} className="ml-4">
              <Play className="mr-2 h-4 w-4" />
              Use Template
            </Button>
          </div>

          {/* Template Stats */}
          <div className="mt-4 flex items-center gap-4">
            <div className="flex items-center gap-1">
              <Star className="h-4 w-4 fill-current text-yellow-500" />
              <span className="font-medium">{template.rating}</span>
            </div>
            <div className="flex items-center gap-1">
              <Download className="h-4 w-4 text-muted-foreground" />
              <span className="text-sm text-muted-foreground">
                {template.usageCount.toLocaleString()} uses
              </span>
            </div>
            <div className="flex items-center gap-1">
              <Clock className="h-4 w-4 text-muted-foreground" />
              <span className="text-sm text-muted-foreground">
                {template.metadata.estimatedDuration}
              </span>
            </div>
            <div className="flex items-center gap-1">
              <Users className="h-4 w-4 text-muted-foreground" />
              <span className="text-sm text-muted-foreground">
                by {template.createdBy}
              </span>
            </div>
          </div>

          {/* Tags and Category */}
          <div className="mt-3 flex items-center gap-2">
            <Badge className="bg-blue-100 text-blue-800">
              {template.category}
            </Badge>
            <Badge className={getComplexityColor(template.metadata.complexity)}>
              {template.metadata.complexity}
            </Badge>
            {template.tags.map((tag) => (
              <Badge key={tag} variant="secondary">
                {tag}
              </Badge>
            ))}
          </div>
        </DialogHeader>

        <Separator className="my-4" />

        <Tabs defaultValue="overview" className="w-full">
          <TabsList className="grid w-full grid-cols-5">
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="workflow">Workflow</TabsTrigger>
            <TabsTrigger value="parameters">Parameters</TabsTrigger>
            <TabsTrigger value="requirements">Requirements</TabsTrigger>
            <TabsTrigger value="documentation">Documentation</TabsTrigger>
          </TabsList>

          <TabsContent value="overview" className="space-y-4">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <Card>
                <CardHeader className="pb-3">
                  <CardTitle className="text-sm font-medium">
                    Template Information
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Version:</span>
                    <span>{template.metadata.version}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Schema:</span>
                    <span>{template.metadata.schemaVersion}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Created:</span>
                    <span>
                      {new Date(template.createdAt).toLocaleDateString()}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Updated:</span>
                    <span>
                      {new Date(template.updatedAt).toLocaleDateString()}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Public:</span>
                    <span>{template.isPublic ? 'Yes' : 'No'}</span>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="pb-3">
                  <CardTitle className="text-sm font-medium">
                    Execution Details
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Tasks:</span>
                    <span>{template.workflow.tasks.length}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">
                      Input Parameters:
                    </span>
                    <span>
                      {template.workflow.inputParameters?.length || 0}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">
                      Output Parameters:
                    </span>
                    <span>
                      {template.workflow.outputParameters?.length || 0}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Timeout:</span>
                    <span>
                      {template.workflow.timeoutSeconds
                        ? `${Math.floor(template.workflow.timeoutSeconds / 60)} minutes`
                        : 'No limit'}
                    </span>
                  </div>
                </CardContent>
              </Card>
            </div>

            {/* Examples */}
            {template.metadata.examples &&
              template.metadata.examples.length > 0 && (
                <Card>
                  <CardHeader>
                    <CardTitle className="text-sm font-medium">
                      Usage Examples
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <div className="space-y-4">
                      {template.metadata.examples.map((example, index) => (
                        <div key={index} className="rounded-lg border p-3">
                          <h4 className="mb-2 text-sm font-medium">
                            {example.name}
                          </h4>
                          <p className="mb-3 text-sm text-muted-foreground">
                            {example.description}
                          </p>
                          <div className="grid grid-cols-1 gap-3 text-xs md:grid-cols-2">
                            <div>
                              <div className="mb-1 font-medium">Input:</div>
                              <pre className="overflow-x-auto rounded bg-muted p-2">
                                {JSON.stringify(example.input, null, 2)}
                              </pre>
                            </div>
                            <div>
                              <div className="mb-1 font-medium">
                                Expected Output:
                              </div>
                              <pre className="overflow-x-auto rounded bg-muted p-2">
                                {JSON.stringify(
                                  example.expectedOutput,
                                  null,
                                  2
                                )}
                              </pre>
                            </div>
                          </div>
                          {example.notes && (
                            <div className="mt-2 text-xs text-muted-foreground">
                              <Info className="mr-1 inline h-3 w-3" />
                              {example.notes}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  </CardContent>
                </Card>
              )}
          </TabsContent>

          <TabsContent value="workflow" className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-medium">
                  Workflow Tasks
                </CardTitle>
                <CardDescription>
                  {template.workflow.tasks.length} tasks in this workflow
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  {template.workflow.tasks.map((task, index) => (
                    <div
                      key={index}
                      className="flex items-start gap-3 rounded-lg border p-3"
                    >
                      <div className="text-lg">
                        {getTaskTypeIcon(task.type)}
                      </div>
                      <div className="flex-1">
                        <div className="mb-1 flex items-center gap-2">
                          <h4 className="text-sm font-medium">{task.name}</h4>
                          <Badge variant="outline" className="text-xs">
                            {task.type}
                          </Badge>
                          {task.optional && (
                            <Badge variant="secondary" className="text-xs">
                              Optional
                            </Badge>
                          )}
                        </div>
                        <p className="text-sm text-muted-foreground">
                          {task.description}
                        </p>
                        {task.configurable && (
                          <div className="mt-2 flex flex-wrap gap-1">
                            {Object.entries(task.configurable).map(
                              ([key, value]) =>
                                value && (
                                  <Badge
                                    key={key}
                                    variant="outline"
                                    className="text-xs"
                                  >
                                    {key}
                                  </Badge>
                                )
                            )}
                          </div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="parameters" className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-medium">
                  Input Parameters
                </CardTitle>
                <CardDescription>
                  Parameters required to instantiate this template
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  {template.parameters.map((param, index) => (
                    <div key={index} className="rounded-lg border p-3">
                      <div className="mb-2 flex items-center gap-2">
                        <h4 className="text-sm font-medium">{param.name}</h4>
                        <Badge variant="outline" className="text-xs">
                          {param.type}
                        </Badge>
                        {param.required && (
                          <Badge variant="destructive" className="text-xs">
                            Required
                          </Badge>
                        )}
                      </div>
                      <p className="mb-2 text-sm text-muted-foreground">
                        {param.description}
                      </p>

                      {param.defaultValue && (
                        <div className="text-xs">
                          <span className="text-muted-foreground">
                            Default:
                          </span>
                          <span className="ml-1 rounded bg-muted px-1 font-mono">
                            {JSON.stringify(param.defaultValue)}
                          </span>
                        </div>
                      )}

                      {param.validation && (
                        <div className="mt-1 text-xs">
                          <span className="text-muted-foreground">
                            Validation:
                          </span>
                          <span className="ml-1">
                            {Object.entries(param.validation).map(
                              ([key, value]) => (
                                <span key={key} className="mr-2">
                                  {key}: {value}
                                </span>
                              )
                            )}
                          </span>
                        </div>
                      )}

                      {param.options && (
                        <div className="mt-2">
                          <div className="mb-1 text-xs text-muted-foreground">
                            Options:
                          </div>
                          <div className="flex flex-wrap gap-1">
                            {param.options.map((option, optIndex) => (
                              <Badge
                                key={optIndex}
                                variant="secondary"
                                className="text-xs"
                              >
                                {option.label}
                              </Badge>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="requirements" className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-medium">
                  Required Services
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-2">
                  {template.metadata.requiredServices.map((service, index) => (
                    <div
                      key={index}
                      className="flex items-center gap-2 text-sm"
                    >
                      <CheckCircle className="h-4 w-4 text-green-600" />
                      <span>{service}</span>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-medium">
                  Supported Formats
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex flex-wrap gap-2">
                  {template.metadata.supportedFormats.map((format, index) => (
                    <Badge key={index} variant="outline">
                      {format}
                    </Badge>
                  ))}
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="documentation" className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-medium">
                  Documentation
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="prose max-w-none text-sm">
                  <p>{template.metadata.documentation}</p>
                </div>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}
