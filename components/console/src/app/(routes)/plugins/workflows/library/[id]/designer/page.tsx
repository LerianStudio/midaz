'use client'

import React, { useState, useCallback, useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { WorkflowCanvas } from '@/components/workflows/designer/workflow-canvas'
import { useWorkflow } from '@/core/application/use-cases/workflows/use-workflow'
import { Workflow } from '@/core/domain/entities/workflow'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { useToast } from '@/hooks/use-toast'
import {
  Save,
  X,
  Play,
  Pause,
  CheckCircle,
  AlertCircle,
  Loader2,
  ArrowLeft
} from 'lucide-react'

interface WorkflowDesignerPageProps {
  params: {
    id: string
  }
}

export default function WorkflowDesignerPage({
  params
}: WorkflowDesignerPageProps) {
  const router = useRouter()
  const { toast } = useToast()
  const workflowId = params.id
  const isNewWorkflow = workflowId === 'new'

  const {
    workflow,
    validationResult,
    isFetching,
    isUpdating,
    isCreating,
    isValidating,
    error,
    saveWorkflow,
    validateWorkflow,
    updateWorkflowStatus,
    setWorkflow
  } = useWorkflow(workflowId)

  const [workflowName, setWorkflowName] = useState('')
  const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false)
  const [isEditingName, setIsEditingName] = useState(false)
  const [templateData, setTemplateData] = useState<any>(null)

  // Load template data from session storage if available
  useEffect(() => {
    if (isNewWorkflow) {
      const storedTemplateData = sessionStorage.getItem(
        `workflow-template-${workflowId}`
      )
      if (storedTemplateData) {
        try {
          const data = JSON.parse(storedTemplateData)
          setTemplateData(data)
          setWorkflowName(data.name || 'New Workflow')
          // Clean up session storage
          sessionStorage.removeItem(`workflow-template-${workflowId}`)
        } catch (e) {
          console.error('Failed to parse template data:', e)
        }
      }
    }
  }, [isNewWorkflow, workflowId])

  // Initialize workflow name when workflow loads
  useEffect(() => {
    if (workflow) {
      setWorkflowName(workflow.name)
    } else if (isNewWorkflow && !templateData) {
      setWorkflowName('New Workflow')
    }
  }, [workflow, isNewWorkflow, templateData])

  // Handle workflow changes from canvas
  const handleWorkflowChange = useCallback(
    (updatedWorkflow: Workflow) => {
      // Update the workflow name if it was changed in the editor
      if (updatedWorkflow.name !== workflowName) {
        setWorkflowName(updatedWorkflow.name)
      }
      setWorkflow(updatedWorkflow)
      setHasUnsavedChanges(true)
    },
    [setWorkflow, workflowName]
  )

  // Save workflow
  const handleSave = useCallback(async () => {
    try {
      // Prepare workflow data
      const workflowData = workflow || {
        name: workflowName,
        description: '',
        tasks: [],
        inputParameters: [],
        metadata: {
          tags: []
        }
      }

      // Update name if changed
      if (workflowName !== workflowData.name) {
        workflowData.name = workflowName
      }

      // Validate before saving
      const validation = await validateWorkflow(workflowData)
      if (!validation || !validation.isValid) {
        toast({
          title: 'Validation Failed',
          description:
            validation?.errors[0]?.message ||
            'Please fix validation errors before saving',
          variant: 'destructive'
        })
        return
      }

      // Save workflow
      const savedWorkflow = await saveWorkflow(workflowData)

      // If this was a new workflow, redirect to the edit page with the new ID
      if (isNewWorkflow && savedWorkflow?.id) {
        router.replace(
          `/plugins/workflows/library/${savedWorkflow.id}/designer`
        )
      }

      setHasUnsavedChanges(false)
      toast({
        title: 'Workflow Saved',
        description: 'Your workflow has been saved successfully'
      })
    } catch (error) {
      console.error('Failed to save workflow:', error)
      toast({
        title: 'Save Failed',
        description:
          error instanceof Error ? error.message : 'Failed to save workflow',
        variant: 'destructive'
      })
    }
  }, [
    workflow,
    workflowName,
    isNewWorkflow,
    saveWorkflow,
    validateWorkflow,
    router
  ])

  // Handle cancel/back
  const handleCancel = useCallback(() => {
    if (hasUnsavedChanges) {
      const confirmed = window.confirm(
        'You have unsaved changes. Are you sure you want to leave?'
      )
      if (!confirmed) return
    }
    router.back()
  }, [hasUnsavedChanges, router])

  // Handle status change
  const handleStatusChange = useCallback(
    async (newStatus: 'ACTIVE' | 'INACTIVE') => {
      if (!workflow || isNewWorkflow) return

      try {
        await updateWorkflowStatus(newStatus)
        toast({
          title: 'Status Updated',
          description: `Workflow is now ${newStatus.toLowerCase()}`
        })
      } catch (error) {
        toast({
          title: 'Update Failed',
          description:
            error instanceof Error ? error.message : 'Failed to update status',
          variant: 'destructive'
        })
      }
    },
    [workflow, isNewWorkflow, updateWorkflowStatus]
  )

  // Loading state
  if (isFetching && !isNewWorkflow) {
    return (
      <div className="h-screen overflow-hidden">
        {/* Header Skeleton */}
        <div className="border-b bg-background">
          <div className="flex h-16 items-center gap-4 px-4">
            <Skeleton className="h-8 w-8" />
            <Skeleton className="h-8 w-48" />
            <Skeleton className="ml-2 h-6 w-16" />
            <div className="ml-auto flex items-center gap-2">
              <Skeleton className="h-9 w-24" />
              <Skeleton className="h-9 w-20" />
            </div>
          </div>
        </div>
        {/* Canvas Skeleton */}
        <div className="flex h-[calc(100vh-4rem)]">
          <Skeleton className="h-full w-80" />
          <div className="flex-1 p-4">
            <Skeleton className="h-full w-full" />
          </div>
        </div>
      </div>
    )
  }

  // Error state
  if (error && !isNewWorkflow) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Card className="max-w-md p-6">
          <div className="mb-4 flex items-center gap-2 text-destructive">
            <AlertCircle className="h-5 w-5" />
            <h2 className="text-lg font-semibold">Failed to Load Workflow</h2>
          </div>
          <p className="mb-4 text-sm text-muted-foreground">{error}</p>
          <div className="flex gap-2">
            <Button variant="outline" onClick={handleCancel}>
              Go Back
            </Button>
            <Button onClick={() => window.location.reload()}>Retry</Button>
          </div>
        </Card>
      </div>
    )
  }

  const currentWorkflow = workflow || {
    id: '',
    name: workflowName,
    description: templateData?.description || '',
    version: 1,
    status: 'DRAFT' as const,
    tasks: templateData?.tasks || [],
    inputParameters: templateData?.inputParameters || [],
    outputParameters: templateData?.outputParameters || [],
    createdBy: '',
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    executionCount: 0,
    successRate: 0,
    metadata: templateData?.metadata || {
      tags: []
    }
  }

  return (
    <div className="flex h-screen flex-col overflow-hidden">
      {/* Header */}
      <div className="border-b bg-background">
        <div className="flex h-16 items-center gap-4 px-4">
          {/* Back Button */}
          <Button
            variant="ghost"
            size="icon"
            onClick={handleCancel}
            title="Back to library"
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>

          {/* Workflow Name */}
          <div className="flex items-center gap-2">
            {isEditingName ? (
              <Input
                value={workflowName}
                onChange={(e) => {
                  setWorkflowName(e.target.value)
                  setHasUnsavedChanges(true)
                }}
                onBlur={() => setIsEditingName(false)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    setIsEditingName(false)
                  }
                }}
                className="h-8 w-64"
                placeholder="Workflow name"
                autoFocus
              />
            ) : (
              <h1
                className="cursor-pointer text-lg font-semibold hover:text-muted-foreground"
                onClick={() => setIsEditingName(true)}
                title="Click to edit name"
              >
                {workflowName}
              </h1>
            )}

            {/* Status Badge */}
            {!isNewWorkflow && workflow && (
              <Badge
                variant={
                  workflow.status === 'ACTIVE'
                    ? 'default'
                    : workflow.status === 'DRAFT'
                      ? 'secondary'
                      : workflow.status === 'INACTIVE'
                        ? 'outline'
                        : 'destructive'
                }
              >
                {workflow.status}
              </Badge>
            )}
          </div>

          {/* Validation Status */}
          {validationResult && (
            <div className="ml-4 flex items-center gap-2">
              {validationResult.isValid ? (
                <div className="flex items-center gap-1 text-green-600">
                  <CheckCircle className="h-4 w-4" />
                  <span className="text-sm">Valid</span>
                </div>
              ) : (
                <div className="flex items-center gap-1 text-destructive">
                  <AlertCircle className="h-4 w-4" />
                  <span className="text-sm">
                    {validationResult.errors.length} error
                    {validationResult.errors.length !== 1 ? 's' : ''}
                  </span>
                </div>
              )}
            </div>
          )}

          {/* Actions */}
          <div className="ml-auto flex items-center gap-2">
            {/* Status Toggle */}
            {!isNewWorkflow && workflow && workflow.status !== 'DRAFT' && (
              <Button
                variant="outline"
                size="sm"
                onClick={() =>
                  handleStatusChange(
                    workflow.status === 'ACTIVE' ? 'INACTIVE' : 'ACTIVE'
                  )
                }
                disabled={isUpdating}
              >
                {workflow.status === 'ACTIVE' ? (
                  <>
                    <Pause className="mr-1 h-4 w-4" />
                    Deactivate
                  </>
                ) : (
                  <>
                    <Play className="mr-1 h-4 w-4" />
                    Activate
                  </>
                )}
              </Button>
            )}

            {/* Cancel Button */}
            <Button
              variant="outline"
              onClick={handleCancel}
              disabled={isUpdating || isCreating}
            >
              <X className="mr-1 h-4 w-4" />
              Cancel
            </Button>

            {/* Save Button */}
            <Button
              onClick={handleSave}
              disabled={
                isUpdating || isCreating || isValidating || !hasUnsavedChanges
              }
            >
              {isUpdating || isCreating ? (
                <>
                  <Loader2 className="mr-1 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                <>
                  <Save className="mr-1 h-4 w-4" />
                  Save
                </>
              )}
            </Button>
          </div>
        </div>
      </div>

      {/* Workflow Canvas */}
      <div className="flex-1">
        <WorkflowCanvas
          workflow={currentWorkflow}
          onWorkflowChange={handleWorkflowChange}
          readonly={false}
        />
      </div>
    </div>
  )
}
