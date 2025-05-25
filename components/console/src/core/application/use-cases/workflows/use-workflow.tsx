'use client'

import { useState, useCallback, useEffect } from 'react'
import { useServerAction } from '@/lib/hooks/use-server-action'
import {
  Workflow,
  CreateWorkflowRequest,
  UpdateWorkflowRequest,
  WorkflowValidationResult,
  WorkflowStatus
} from '@/core/domain/entities/workflow'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import {
  createWorkflowAction,
  fetchWorkflowByIdAction,
  updateWorkflowAction,
  deleteWorkflowAction,
  validateWorkflowAction,
  duplicateWorkflowAction,
  updateWorkflowStatusAction
} from './workflow-server-actions'

export function useWorkflow(workflowId?: string) {
  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [validationResult, setValidationResult] =
    useState<WorkflowValidationResult | null>(null)

  const {
    execute: executeCreate,
    isPending: isCreating,
    error: createError
  } = useServerAction(createWorkflowAction)

  const {
    execute: executeFetch,
    isPending: isFetching,
    error: fetchError
  } = useServerAction(fetchWorkflowByIdAction)

  const {
    execute: executeUpdate,
    isPending: isUpdating,
    error: updateError
  } = useServerAction(updateWorkflowAction)

  const {
    execute: executeDelete,
    isPending: isDeleting,
    error: deleteError
  } = useServerAction(deleteWorkflowAction)

  const {
    execute: executeValidate,
    isPending: isValidating,
    error: validateError
  } = useServerAction(validateWorkflowAction)

  const {
    execute: executeDuplicate,
    isPending: isDuplicating,
    error: duplicateError
  } = useServerAction(duplicateWorkflowAction)

  const {
    execute: executeUpdateStatus,
    isPending: isUpdatingStatus,
    error: updateStatusError
  } = useServerAction(updateWorkflowStatusAction)

  // Fetch workflow on mount if ID is provided and not 'new'
  useEffect(() => {
    if (workflowId && workflowId !== 'new') {
      executeFetch({ workflowId }).then((result) => {
        if (result.success) {
          setWorkflow(result.data)
        }
      })
    }
  }, [workflowId, executeFetch])

  const createWorkflow = useCallback(
    async (workflowData: CreateWorkflowRequest) => {
      const result = await executeCreate({ workflow: workflowData })
      if (result.success) {
        setWorkflow(result.data)
        return result.data
      }
      throw new Error(result.error || 'Failed to create workflow')
    },
    [executeCreate]
  )

  const updateWorkflow = useCallback(
    async (workflowData: UpdateWorkflowRequest) => {
      if (!workflowId || workflowId === 'new') {
        throw new Error('Cannot update workflow without ID')
      }

      const result = await executeUpdate({
        workflowId,
        workflow: workflowData
      })

      if (result.success) {
        setWorkflow(result.data)
        return result.data
      }
      throw new Error(result.error || 'Failed to update workflow')
    },
    [workflowId, executeUpdate]
  )

  const deleteWorkflow = useCallback(async () => {
    if (!workflowId || workflowId === 'new') {
      throw new Error('Cannot delete workflow without ID')
    }

    const result = await executeDelete({ workflowId })
    if (result.success) {
      setWorkflow(null)
      return
    }
    throw new Error(result.error || 'Failed to delete workflow')
  }, [workflowId, executeDelete])

  const validateWorkflow = useCallback(
    async (workflowData: Workflow | CreateWorkflowRequest) => {
      const result = await executeValidate({ workflow: workflowData })
      if (result.success) {
        setValidationResult(result.data)
        return result.data
      }
      throw new Error(result.error || 'Failed to validate workflow')
    },
    [executeValidate]
  )

  const duplicateWorkflow = useCallback(
    async (name: string) => {
      if (!workflowId || workflowId === 'new') {
        throw new Error('Cannot duplicate workflow without ID')
      }

      const result = await executeDuplicate({ workflowId, name })
      if (result.success) {
        return result.data
      }
      throw new Error(result.error || 'Failed to duplicate workflow')
    },
    [workflowId, executeDuplicate]
  )

  const updateWorkflowStatus = useCallback(
    async (status: WorkflowStatus) => {
      if (!workflowId || workflowId === 'new') {
        throw new Error('Cannot update status without workflow ID')
      }

      const result = await executeUpdateStatus({ workflowId, status })
      if (result.success) {
        setWorkflow(result.data)
        return result.data
      }
      throw new Error(result.error || 'Failed to update workflow status')
    },
    [workflowId, executeUpdateStatus]
  )

  const saveWorkflow = useCallback(
    async (workflowData: Workflow | CreateWorkflowRequest) => {
      if (workflowId && workflowId !== 'new') {
        // Update existing workflow
        return updateWorkflow(workflowData as UpdateWorkflowRequest)
      } else {
        // Create new workflow
        return createWorkflow(workflowData as CreateWorkflowRequest)
      }
    },
    [workflowId, createWorkflow, updateWorkflow]
  )

  return {
    workflow,
    validationResult,
    isLoading:
      isFetching ||
      isCreating ||
      isUpdating ||
      isDeleting ||
      isValidating ||
      isDuplicating ||
      isUpdatingStatus,
    isCreating,
    isFetching,
    isUpdating,
    isDeleting,
    isValidating,
    isDuplicating,
    isUpdatingStatus,
    error:
      createError ||
      fetchError ||
      updateError ||
      deleteError ||
      validateError ||
      duplicateError ||
      updateStatusError,
    createWorkflow,
    updateWorkflow,
    deleteWorkflow,
    validateWorkflow,
    duplicateWorkflow,
    updateWorkflowStatus,
    saveWorkflow,
    setWorkflow
  }
}
