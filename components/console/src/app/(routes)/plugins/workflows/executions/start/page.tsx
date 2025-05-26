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

// Dynamic schema based on workflow parameters
const createExecutionSchema = (
  parameters: Array<{ name: string; type: string; required: boolean }>
) => {
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
    } else {
      schema = schema.min(1, 'This field is required')
    }

    shape[param.name] = schema
  })

  return z.object(shape)
}

export default function StartExecutionPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const workflowId = searchParams.get('workflowId')

  const [workflow, setWorkflow] = useState<Workflow | null>(null)
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

      setWorkflow(result.data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load workflow')
    } finally {
      setLoading(false)
    }
  }

  // Initialize form
  const form = useForm({
    resolver: workflow?.inputParameters
      ? zodResolver(createExecutionSchema(workflow.inputParameters))
      : undefined,
    defaultValues:
      workflow?.inputParameters?.reduce(
        (acc, param) => ({
          ...acc,
          [param.name]: param.defaultValue || ''
        }),
        {}
      ) || {}
  })

  const onSubmit = async (values: Record<string, any>) => {
    if (!workflow) return

    try {
      setSubmitting(true)

      // Convert JSON strings to objects for object-type parameters
      const processedValues = { ...values }
      workflow.inputParameters?.forEach((param) => {
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

      const result = await startWorkflowExecution({
        workflowId: workflow.id,
        workflowName: workflow.name,
        workflowVersion: workflow.version,
        input: processedValues,
        metadata: {
          startedBy: 'user@example.com', // TODO: Get from auth context
          startedFrom: 'console'
        }
      })

      if (!result.success || !result.data) {
        throw new Error(result.error || 'Failed to start execution')
      }

      toast({
        title: 'Execution Started',
        description: `Workflow "${workflow.name}" is now running`
      })

      // Redirect to execution details
      router.push(`/plugins/workflows/executions/${result.data.id}`)
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

  const renderParameterField = (param: any) => {
    return (
      <FormField
        key={param.name}
        control={form.control}
        name={param.name}
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {param.label || param.name}
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
                    <SelectValue
                      placeholder={`Select ${param.label || param.name}`}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {param.options.map((option: any) => (
                      <SelectItem key={option.value} value={option.value}>
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
                <Input
                  {...field}
                  placeholder={
                    param.placeholder || `Enter ${param.label || param.name}`
                  }
                />
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
              {workflow.inputParameters &&
              workflow.inputParameters.length > 0 ? (
                <>
                  <div className="space-y-4">
                    {workflow.inputParameters.map(renderParameterField)}
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
                    This workflow doesn't require any input parameters.
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
