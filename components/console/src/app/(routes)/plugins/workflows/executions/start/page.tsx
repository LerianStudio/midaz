'use client'

import React, { useEffect, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import * as z from 'zod'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { ArrowLeft, Play, AlertCircle, Loader2 } from 'lucide-react'
import { Workflow } from '@/core/domain/entities/workflow'
import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'
import {
  getWorkflowById,
  startWorkflowExecution
} from '@/app/actions/workflows'
import { toast } from '@/hooks/use-toast'
import { TemplateParameter } from '@/core/domain/entities/workflow-template'

// Dynamic schema based on workflow parameters
const createExecutionSchema = (parameters: TemplateParameter[]) => {
  const shape: Record<string, z.ZodTypeAny> = {}

  parameters.forEach((param) => {
    let schema: z.ZodTypeAny

    switch (param.type) {
      case 'number':
        schema = z.coerce.number()
        break
      case 'boolean':
        schema = z.coerce.boolean()
        break
      case 'object':
        schema = z.string().refine((val) => {
          try {
            JSON.parse(val)
            return true
          } catch {
            return false
          }
        }, 'Must be valid JSON')
        break
      default:
        schema = z.string()
    }

    if (!param.required) {
      schema = schema.optional()
    } else if (param.type !== 'boolean' && param.type !== 'number') {
      schema = (schema as z.ZodString).min(1, 'This field is required')
    }

    shape[param.name] = schema
  })

  return z.object(shape)
}

interface ExtendedWorkflow extends Workflow {
  parameters?: TemplateParameter[]
}

type WorkflowFormData = Record<string, any>

export default function StartExecutionPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const workflowId = searchParams.get('workflowId')

  const [workflow, setWorkflow] = useState<ExtendedWorkflow | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  // Load workflow data
  useEffect(() => {
    if (!workflowId) {
      setError('No workflow ID provided')
      setLoading(false)
      return
    }

    loadWorkflow(workflowId)
  }, [workflowId])

  const loadWorkflow = async (id: string) => {
    try {
      setLoading(true)
      const result = await getWorkflowById(id)

      if (!result.success || !result.data) {
        throw new Error(result.error || 'Failed to load workflow')
      }

      // For demo purposes, create mock parameters based on inputParameters
      const extendedWorkflow: ExtendedWorkflow = {
        ...result.data,
        parameters: result.data.inputParameters?.map((param) => ({
          name: param,
          type: 'string',
          required: true,
          description: `Parameter ${param}`
        }))
      }
      setWorkflow(extendedWorkflow)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load workflow')
    } finally {
      setLoading(false)
    }
  }

  // Initialize form
  const form = useForm<WorkflowFormData>({
    resolver: workflow?.parameters
      ? zodResolver(createExecutionSchema(workflow.parameters))
      : undefined,
    defaultValues:
      workflow?.parameters?.reduce(
        (acc, param) => ({
          ...acc,
          [param.name]: param.defaultValue || ''
        }),
        {} as WorkflowFormData
      ) || {}
  })

  const onSubmit = async (values: Record<string, any>) => {
    if (!workflow) return

    try {
      setSubmitting(true)

      // Convert JSON strings to objects for object-type parameters
      const processedValues = { ...values }
      workflow.parameters?.forEach((param) => {
        if (param.type === 'object' && processedValues[param.name]) {
          try {
            processedValues[param.name] = JSON.parse(
              processedValues[param.name]
            )
          } catch {
            // Keep as string if parsing fails
          }
        }
      })

      const result = await startWorkflowExecution(workflow.id, processedValues)

      if (!result.success || !result.data) {
        throw new Error(result.error || 'Failed to start execution')
      }

      toast({
        title: 'Execution Started',
        description: `Workflow "${workflow.name}" is now running`
      })

      // Redirect to execution details
      router.push(`/plugins/workflows/executions/${result.data.executionId}`)
    } catch (err) {
      toast({
        variant: 'destructive',
        title: 'Failed to start execution',
        description:
          err instanceof Error ? err.message : 'Unknown error occurred'
      })
    } finally {
      setSubmitting(false)
    }
  }

  if (loading) {
    return (
      <div className="container max-w-4xl py-8">
        <Skeleton className="mb-4 h-8 w-64" />
        <Card>
          <CardHeader>
            <Skeleton className="h-6 w-48" />
            <Skeleton className="mt-2 h-4 w-96" />
          </CardHeader>
          <CardContent className="space-y-4">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </CardContent>
        </Card>
      </div>
    )
  }

  if (error || !workflow) {
    return (
      <div className="container max-w-4xl py-8">
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{error || 'Workflow not found'}</AlertDescription>
        </Alert>
        <Button
          variant="outline"
          onClick={() => router.push('/plugins/workflows/library')}
          className="mt-4"
        >
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Library
        </Button>
      </div>
    )
  }

  const renderParameterField = (param: TemplateParameter) => {
    return (
      <FormField
        key={param.name}
        control={form.control}
        name={param.name as keyof WorkflowFormData}
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {param.name}
              {param.required && <span className="ml-1 text-red-500">*</span>}
            </FormLabel>
            {param.description && (
              <FormDescription>{param.description}</FormDescription>
            )}
            <FormControl>
              {param.type === 'select' && param.options ? (
                <Select
                  onValueChange={field.onChange}
                  defaultValue={field.value}
                >
                  <SelectTrigger>
                    <SelectValue placeholder={`Select ${param.name}`} />
                  </SelectTrigger>
                  <SelectContent>
                    {param.options.map((option) => (
                      <SelectItem
                        key={option.value}
                        value={String(option.value)}
                      >
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : param.type === 'object' ? (
                <Textarea
                  {...field}
                  placeholder='{"key": "value"}'
                  className="font-mono"
                  rows={5}
                />
              ) : param.type === 'number' ? (
                <Input {...field} type="number" placeholder="0" />
              ) : (
                <Input {...field} placeholder={`Enter ${param.name}`} />
              )}
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    )
  }

  return (
    <div className="container max-w-4xl py-8">
      <div className="mb-6">
        <Button variant="ghost" onClick={() => router.back()} className="mb-4">
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back
        </Button>

        <h1 className="text-3xl font-bold">Start Workflow Execution</h1>
        <p className="mt-2 text-muted-foreground">
          Configure parameters and start a new execution of this workflow
        </p>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>{workflow.name}</CardTitle>
              <CardDescription className="mt-2">
                {workflow.description || 'No description available'}
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Badge
                variant={workflow.status === 'ACTIVE' ? 'default' : 'secondary'}
              >
                {workflow.status}
              </Badge>
              <Badge variant="outline">v{workflow.version}</Badge>
            </div>
          </div>
        </CardHeader>

        <Separator />

        <CardContent className="pt-6">
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
              {workflow.parameters && workflow.parameters.length > 0 ? (
                <>
                  <div className="space-y-4">
                    {workflow.parameters.map(renderParameterField)}
                  </div>

                  <Alert>
                    <AlertCircle className="h-4 w-4" />
                    <AlertDescription>
                      Parameters marked with{' '}
                      <span className="text-red-500">*</span> are required.
                      Object parameters should be valid JSON.
                    </AlertDescription>
                  </Alert>
                </>
              ) : (
                <Alert>
                  <AlertDescription>
                    This workflow doesn&apos;t require any input parameters.
                  </AlertDescription>
                </Alert>
              )}

              <div className="flex justify-end gap-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => router.back()}
                  disabled={submitting}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={submitting}>
                  {submitting ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Starting...
                    </>
                  ) : (
                    <>
                      <Play className="mr-2 h-4 w-4" />
                      Start Execution
                    </>
                  )}
                </Button>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  )
}
