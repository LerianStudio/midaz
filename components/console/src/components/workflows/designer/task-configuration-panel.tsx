'use client'

import { useState } from 'react'
import { Node } from 'reactflow'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import {
  X,
  Save,
  Trash2,
  Plus,
  Minus,
  Settings,
  Code,
  Clock,
  AlertTriangle
} from 'lucide-react'
import { WorkflowTask, TaskType } from '@/core/domain/entities/workflow'

interface TaskConfigurationPanelProps {
  node: Node
  onUpdateConfig: (config: Partial<WorkflowTask>) => void
  onDelete: () => void
  onClose: () => void
}

export function TaskConfigurationPanel({
  node,
  onUpdateConfig,
  onDelete,
  onClose
}: TaskConfigurationPanelProps) {
  const [config, setConfig] = useState<WorkflowTask>(node.data.config)
  const [newParamKey, setNewParamKey] = useState('')
  const [newParamValue, setNewParamValue] = useState('')

  const updateConfig = (updates: Partial<WorkflowTask>) => {
    const newConfig = { ...config, ...updates }
    setConfig(newConfig)
    onUpdateConfig(updates)
  }

  const addInputParameter = () => {
    if (newParamKey.trim()) {
      const newInputParameters = {
        ...config.inputParameters,
        [newParamKey.trim()]: newParamValue || ''
      }
      updateConfig({ inputParameters: newInputParameters })
      setNewParamKey('')
      setNewParamValue('')
    }
  }

  const removeInputParameter = (key: string) => {
    const newInputParameters = { ...config.inputParameters }
    delete newInputParameters[key]
    updateConfig({ inputParameters: newInputParameters })
  }

  const updateInputParameter = (key: string, value: any) => {
    const newInputParameters = {
      ...config.inputParameters,
      [key]: value
    }
    updateConfig({ inputParameters: newInputParameters })
  }

  const renderBasicConfiguration = () => (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="task-name">Task Name</Label>
        <Input
          id="task-name"
          value={config.name}
          onChange={(e) => updateConfig({ name: e.target.value })}
          placeholder="Enter task name"
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="reference-name">Reference Name</Label>
        <Input
          id="reference-name"
          value={config.taskReferenceName}
          onChange={(e) => updateConfig({ taskReferenceName: e.target.value })}
          placeholder="Unique reference name"
        />
        <p className="text-xs text-muted-foreground">
          Used to reference this task&apos;s output in other tasks
        </p>
      </div>

      <div className="space-y-2">
        <Label htmlFor="description">Description (Optional)</Label>
        <Textarea
          id="description"
          value={config.description || ''}
          onChange={(e) => updateConfig({ description: e.target.value })}
          placeholder="Describe what this task does"
          rows={3}
        />
      </div>

      <div className="space-y-3">
        <div className="flex items-center space-x-2">
          <Checkbox
            id="optional"
            checked={config.optional || false}
            onCheckedChange={(checked) => updateConfig({ optional: checked === true })}
          />
          <Label htmlFor="optional" className="text-sm">
            Optional Task
          </Label>
        </div>

        <div className="flex items-center space-x-2">
          <Checkbox
            id="async-complete"
            checked={config.asyncComplete || false}
            onCheckedChange={(checked) =>
              updateConfig({ asyncComplete: checked === true })
            }
          />
          <Label htmlFor="async-complete" className="text-sm">
            Async Complete
          </Label>
        </div>
      </div>
    </div>
  )

  const renderRetryConfiguration = () => (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="retry-count">Retry Count</Label>
        <Input
          id="retry-count"
          type="number"
          min="0"
          max="10"
          value={config.retryCount || 0}
          onChange={(e) =>
            updateConfig({ retryCount: parseInt(e.target.value) || 0 })
          }
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="timeout">Timeout (seconds)</Label>
        <Input
          id="timeout"
          type="number"
          min="1"
          value={config.timeoutSeconds || ''}
          onChange={(e) =>
            updateConfig({
              timeoutSeconds: parseInt(e.target.value) || undefined
            })
          }
          placeholder="No timeout"
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="start-delay">Start Delay (seconds)</Label>
        <Input
          id="start-delay"
          type="number"
          min="0"
          value={config.startDelay || ''}
          onChange={(e) =>
            updateConfig({ startDelay: parseInt(e.target.value) || undefined })
          }
          placeholder="No delay"
        />
      </div>

      {config.retryCount && config.retryCount > 0 && (
        <div className="space-y-2">
          <Label>Retry Logic</Label>
          <Select
            value={config.retryLogic?.retryPolicy || 'FIXED'}
            onValueChange={(value) =>
              updateConfig({
                retryLogic: {
                  ...config.retryLogic,
                  retryPolicy: value as 'FIXED' | 'EXPONENTIAL_BACKOFF',
                  maxRetries: config.retryCount || 1
                }
              })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="FIXED">Fixed Delay</SelectItem>
              <SelectItem value="EXPONENTIAL_BACKOFF">
                Exponential Backoff
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
      )}
    </div>
  )

  const renderInputParameters = () => (
    <div className="space-y-4">
      <div className="space-y-3">
        <Label>Input Parameters</Label>

        {/* Existing Parameters */}
        <div className="space-y-2">
          {Object.entries(config.inputParameters || {}).map(([key, value]) => (
            <div
              key={key}
              className="flex items-center space-x-2 rounded border p-2"
            >
              <div className="grid flex-1 grid-cols-2 gap-2">
                <Input
                  value={key}
                  onChange={(e) => {
                    const newKey = e.target.value
                    if (newKey !== key) {
                      const newParams = { ...config.inputParameters }
                      delete newParams[key]
                      newParams[newKey] = value
                      updateConfig({ inputParameters: newParams })
                    }
                  }}
                  placeholder="Parameter name"
                  className="text-sm"
                />
                <Input
                  value={
                    typeof value === 'string' ? value : JSON.stringify(value)
                  }
                  onChange={(e) => {
                    try {
                      const parsedValue = JSON.parse(e.target.value)
                      updateInputParameter(key, parsedValue)
                    } catch {
                      updateInputParameter(key, e.target.value)
                    }
                  }}
                  placeholder="Parameter value"
                  className="text-sm"
                />
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => removeInputParameter(key)}
                className="h-8 w-8 p-0 text-red-600"
              >
                <Minus className="h-3 w-3" />
              </Button>
            </div>
          ))}
        </div>

        {/* Add New Parameter */}
        <div className="space-y-2 rounded-lg border-2 border-dashed p-3">
          <Label className="text-sm">Add Parameter</Label>
          <div className="flex items-center space-x-2">
            <Input
              value={newParamKey}
              onChange={(e) => setNewParamKey(e.target.value)}
              placeholder="Parameter name"
              className="text-sm"
            />
            <Input
              value={newParamValue}
              onChange={(e) => setNewParamValue(e.target.value)}
              placeholder="Parameter value"
              className="text-sm"
            />
            <Button
              onClick={addInputParameter}
              disabled={!newParamKey.trim()}
              size="sm"
              className="flex-shrink-0"
            >
              <Plus className="h-3 w-3" />
            </Button>
          </div>
        </div>
      </div>

      {/* Common Parameter Templates */}
      {config.type === 'HTTP' && (
        <div className="space-y-2">
          <Label className="text-sm">HTTP Configuration Templates</Label>
          <div className="grid grid-cols-2 gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() =>
                updateConfig({
                  inputParameters: {
                    ...config.inputParameters,
                    http_request: {
                      uri: 'https://api.example.com/endpoint',
                      method: 'GET',
                      headers: {
                        'Content-Type': 'application/json'
                      }
                    }
                  }
                })
              }
            >
              GET Request
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() =>
                updateConfig({
                  inputParameters: {
                    ...config.inputParameters,
                    http_request: {
                      uri: 'https://api.example.com/endpoint',
                      method: 'POST',
                      headers: {
                        'Content-Type': 'application/json'
                      },
                      body: {}
                    }
                  }
                })
              }
            >
              POST Request
            </Button>
          </div>
        </div>
      )}
    </div>
  )

  const getTaskTypeInfo = () => {
    const taskTypeDescriptions: Record<TaskType, string> = {
      HTTP: 'Make HTTP requests to external services',
      SWITCH: 'Route workflow based on input values',
      DECISION: 'Evaluate conditions and branch accordingly',
      FORK_JOIN: 'Execute tasks in parallel',
      FORK_JOIN_DYNAMIC: 'Execute dynamic parallel tasks',
      JOIN: 'Join parallel task execution',
      SUB_WORKFLOW: 'Execute another workflow',
      WAIT: 'Pause execution for a duration',
      HUMAN: 'Require human intervention',
      TERMINATE: 'End workflow execution',
      LAMBDA: 'Execute serverless function',
      EVENT: 'Publish or wait for events',
      KAFKA_PUBLISH: 'Publish messages to Kafka',
      JSON_JQ_TRANSFORM: 'Transform JSON data',
      SET_VARIABLE: 'Set workflow variables',
      CUSTOM: 'Custom task implementation'
    }

    return taskTypeDescriptions[config.type] || 'Custom task configuration'
  }

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b p-4">
        <div>
          <h3 className="font-semibold">Task Configuration</h3>
          <p className="text-sm text-muted-foreground">{getTaskTypeInfo()}</p>
        </div>
        <Button variant="ghost" size="sm" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      {/* Task Type Badge */}
      <div className="border-b p-4">
        <Badge variant="secondary" className="flex items-center space-x-1">
          <Settings className="h-3 w-3" />
          <span>{config.type}</span>
        </Badge>
      </div>

      {/* Configuration Tabs */}
      <ScrollArea className="flex-1">
        <div className="p-4">
          <Tabs defaultValue="basic" className="w-full">
            <TabsList className="grid w-full grid-cols-3">
              <TabsTrigger value="basic" className="text-xs">
                Basic
              </TabsTrigger>
              <TabsTrigger value="parameters" className="text-xs">
                Parameters
              </TabsTrigger>
              <TabsTrigger value="advanced" className="text-xs">
                Advanced
              </TabsTrigger>
            </TabsList>

            <TabsContent value="basic" className="mt-4">
              {renderBasicConfiguration()}
            </TabsContent>

            <TabsContent value="parameters" className="mt-4">
              {renderInputParameters()}
            </TabsContent>

            <TabsContent value="advanced" className="mt-4">
              {renderRetryConfiguration()}
            </TabsContent>
          </Tabs>
        </div>
      </ScrollArea>

      {/* Actions */}
      <div className="border-t bg-muted/30 p-4">
        <div className="flex items-center justify-between">
          <Button
            variant="destructive"
            size="sm"
            onClick={onDelete}
            className="flex items-center space-x-2"
          >
            <Trash2 className="h-3 w-3" />
            <span>Delete Task</span>
          </Button>
          <div className="flex items-center space-x-2">
            <Button variant="outline" size="sm" onClick={onClose}>
              Cancel
            </Button>
            <Button size="sm">
              <Save className="mr-2 h-3 w-3" />
              Save
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
