'use client'

import { useState } from 'react'
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
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { X, Save, Plus, Minus } from 'lucide-react'
import { Workflow } from '@/core/domain/entities/workflow'

interface WorkflowMetadataEditorProps {
  workflow?: Workflow
  onWorkflowChange?: (workflow: Workflow) => void
  onClose: () => void
}

export function WorkflowMetadataEditor({
  workflow,
  onWorkflowChange,
  onClose
}: WorkflowMetadataEditorProps) {
  const [metadata, setMetadata] = useState({
    name: workflow?.name || '',
    description: workflow?.description || '',
    version: workflow?.version || 1,
    inputParameters: workflow?.inputParameters || [],
    outputParameters: workflow?.outputParameters || [],
    tags: workflow?.metadata.tags || [],
    category: workflow?.metadata.category || '',
    timeoutSeconds: workflow?.metadata.timeoutPolicy?.timeoutSeconds || '',
    ownerEmail: workflow?.metadata.ownerEmail || ''
  })

  const [newInputParam, setNewInputParam] = useState('')
  const [newOutputParam, setNewOutputParam] = useState('')
  const [newTag, setNewTag] = useState('')

  const updateMetadata = (updates: Partial<typeof metadata>) => {
    const newMetadata = { ...metadata, ...updates }
    setMetadata(newMetadata)
  }

  const addInputParameter = () => {
    if (
      newInputParam.trim() &&
      !metadata.inputParameters.includes(newInputParam.trim())
    ) {
      updateMetadata({
        inputParameters: [...metadata.inputParameters, newInputParam.trim()]
      })
      setNewInputParam('')
    }
  }

  const removeInputParameter = (param: string) => {
    updateMetadata({
      inputParameters: metadata.inputParameters.filter((p) => p !== param)
    })
  }

  const addOutputParameter = () => {
    if (
      newOutputParam.trim() &&
      !metadata.outputParameters.includes(newOutputParam.trim())
    ) {
      updateMetadata({
        outputParameters: [...metadata.outputParameters, newOutputParam.trim()]
      })
      setNewOutputParam('')
    }
  }

  const removeOutputParameter = (param: string) => {
    updateMetadata({
      outputParameters: metadata.outputParameters.filter((p) => p !== param)
    })
  }

  const addTag = () => {
    if (newTag.trim() && !metadata.tags.includes(newTag.trim())) {
      updateMetadata({
        tags: [...metadata.tags, newTag.trim()]
      })
      setNewTag('')
    }
  }

  const removeTag = (tag: string) => {
    updateMetadata({
      tags: metadata.tags.filter((t) => t !== tag)
    })
  }

  const handleSave = () => {
    if (workflow && onWorkflowChange) {
      const updatedWorkflow: Workflow = {
        ...workflow,
        name: metadata.name,
        description: metadata.description,
        version: metadata.version,
        inputParameters: metadata.inputParameters,
        outputParameters: metadata.outputParameters,
        metadata: {
          ...workflow.metadata,
          tags: metadata.tags,
          category: metadata.category,
          timeoutPolicy: metadata.timeoutSeconds
            ? {
                timeoutSeconds: parseInt(metadata.timeoutSeconds.toString())
              }
            : undefined,
          ownerEmail: metadata.ownerEmail
        }
      }
      onWorkflowChange(updatedWorkflow)
    }
    onClose()
  }

  return (
    <Card className="flex max-h-[80vh] w-full flex-col overflow-hidden">
      <CardHeader className="flex-shrink-0 pb-3">
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-sm sm:text-base">
              Workflow Metadata
            </CardTitle>
            <CardDescription className="text-xs sm:text-sm">
              Configure workflow properties and settings
            </CardDescription>
          </div>
          <Button variant="ghost" size="sm" onClick={onClose}>
            <X className="h-4 w-4" />
          </Button>
        </div>
      </CardHeader>

      <CardContent className="flex-1 space-y-4 overflow-y-auto p-3 sm:space-y-6 sm:p-6">
        {/* Basic Information */}
        <div className="space-y-3 sm:space-y-4">
          <div className="space-y-2">
            <Label htmlFor="workflow-name" className="text-xs sm:text-sm">
              Workflow Name
            </Label>
            <Input
              id="workflow-name"
              value={metadata.name}
              onChange={(e) => updateMetadata({ name: e.target.value })}
              placeholder="Enter workflow name"
              className="text-sm"
            />
          </div>

          <div className="space-y-2">
            <Label
              htmlFor="workflow-description"
              className="text-xs sm:text-sm"
            >
              Description
            </Label>
            <Textarea
              id="workflow-description"
              value={metadata.description}
              onChange={(e) => updateMetadata({ description: e.target.value })}
              placeholder="Describe what this workflow does"
              rows={3}
              className="resize-none text-sm"
            />
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 sm:gap-4">
            <div className="space-y-2">
              <Label htmlFor="workflow-version" className="text-xs sm:text-sm">
                Version
              </Label>
              <Input
                id="workflow-version"
                type="number"
                min="1"
                value={metadata.version}
                onChange={(e) =>
                  updateMetadata({ version: parseInt(e.target.value) || 1 })
                }
                className="text-sm"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="workflow-category" className="text-xs sm:text-sm">
                Category
              </Label>
              <Input
                id="workflow-category"
                value={metadata.category}
                onChange={(e) => updateMetadata({ category: e.target.value })}
                placeholder="e.g., payments, onboarding"
                className="text-sm"
              />
            </div>
          </div>
        </div>

        <Separator />

        {/* Input Parameters */}
        <div className="space-y-3">
          <Label className="text-xs sm:text-sm">Input Parameters</Label>
          <div className="max-h-32 space-y-2 overflow-y-auto">
            {metadata.inputParameters.map((param) => (
              <div
                key={param}
                className="flex items-center justify-between rounded border p-1.5 sm:p-2"
              >
                <span className="truncate font-mono text-xs sm:text-sm">
                  {param}
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => removeInputParameter(param)}
                  className="h-5 w-5 p-0 text-red-600 sm:h-6 sm:w-6"
                >
                  <Minus className="h-3 w-3" />
                </Button>
              </div>
            ))}
          </div>
          <div className="flex items-center gap-2">
            <Input
              value={newInputParam}
              onChange={(e) => setNewInputParam(e.target.value)}
              placeholder="Parameter name"
              className="text-xs sm:text-sm"
              onKeyPress={(e) => e.key === 'Enter' && addInputParameter()}
            />
            <Button
              onClick={addInputParameter}
              disabled={!newInputParam.trim()}
              size="sm"
              className="flex-shrink-0"
            >
              <Plus className="h-3 w-3" />
            </Button>
          </div>
        </div>

        <Separator />

        {/* Output Parameters */}
        <div className="space-y-3">
          <Label>Output Parameters</Label>
          <div className="space-y-2">
            {metadata.outputParameters.map((param) => (
              <div
                key={param}
                className="flex items-center justify-between rounded border p-2"
              >
                <span className="font-mono text-sm">{param}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => removeOutputParameter(param)}
                  className="h-6 w-6 p-0 text-red-600"
                >
                  <Minus className="h-3 w-3" />
                </Button>
              </div>
            ))}
          </div>
          <div className="flex items-center space-x-2">
            <Input
              value={newOutputParam}
              onChange={(e) => setNewOutputParam(e.target.value)}
              placeholder="Parameter name"
              className="text-sm"
              onKeyPress={(e) => e.key === 'Enter' && addOutputParameter()}
            />
            <Button
              onClick={addOutputParameter}
              disabled={!newOutputParam.trim()}
              size="sm"
            >
              <Plus className="h-3 w-3" />
            </Button>
          </div>
        </div>

        <Separator />

        {/* Tags */}
        <div className="space-y-3">
          <Label className="text-xs sm:text-sm">Tags</Label>
          <div className="flex flex-wrap gap-1.5 sm:gap-2">
            {metadata.tags.map((tag) => (
              <Badge
                key={tag}
                variant="secondary"
                className="flex items-center gap-1 text-[10px] sm:text-xs"
              >
                <span>{tag}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => removeTag(tag)}
                  className="h-3 w-3 p-0 hover:bg-transparent sm:h-4 sm:w-4"
                >
                  <X className="h-2 w-2" />
                </Button>
              </Badge>
            ))}
          </div>
          <div className="flex items-center gap-2">
            <Input
              value={newTag}
              onChange={(e) => setNewTag(e.target.value)}
              placeholder="Add tag"
              className="text-xs sm:text-sm"
              onKeyPress={(e) => e.key === 'Enter' && addTag()}
            />
            <Button
              onClick={addTag}
              disabled={!newTag.trim()}
              size="sm"
              className="flex-shrink-0"
            >
              <Plus className="h-3 w-3" />
            </Button>
          </div>
        </div>

        <Separator />

        {/* Advanced Settings */}
        <div className="space-y-4">
          <Label>Advanced Settings</Label>

          <div className="space-y-2">
            <Label htmlFor="timeout">Timeout (seconds)</Label>
            <Input
              id="timeout"
              type="number"
              min="1"
              value={metadata.timeoutSeconds}
              onChange={(e) =>
                updateMetadata({ timeoutSeconds: e.target.value })
              }
              placeholder="No timeout"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="owner-email">Owner Email</Label>
            <Input
              id="owner-email"
              type="email"
              value={metadata.ownerEmail}
              onChange={(e) => updateMetadata({ ownerEmail: e.target.value })}
              placeholder="owner@company.com"
            />
          </div>
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-2 border-t pt-4">
          <Button
            variant="outline"
            onClick={onClose}
            size="sm"
            className="text-xs sm:text-sm"
          >
            Cancel
          </Button>
          <Button onClick={handleSave} size="sm" className="text-xs sm:text-sm">
            <Save className="mr-2 h-3 w-3" />
            Save Changes
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
