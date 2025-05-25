'use client'

import React, { useState } from 'react'
import { z } from 'zod'
import { useForm, UseFormReturn } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import {
  ChevronLeft,
  ChevronRight,
  Check,
  FileText,
  Settings,
  Eye
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { Form } from '@/components/ui/form'
import { InputField, SelectField } from '@/components/form'
import { Textarea } from '@/components/ui/textarea'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage
} from '@/components/ui/form'
import { MetadataField } from '@/components/form/metadata-field'
import { Badge } from '@/components/ui/badge'
import { TemplateCatalog } from '@/components/workflows/templates/template-catalog'
import { WorkflowTemplate } from '@/core/domain/entities/workflow-template'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { InfoIcon } from 'lucide-react'

interface WorkflowCreationWizardProps {
  onSubmit: (data: WorkflowFormData) => void
  onCancel: () => void
  initialTemplate?: WorkflowTemplate
  initialTemplateParameters?: Record<string, any>
}

// Form validation schemas
const basicInfoSchema = z.object({
  name: z.string().min(3, 'Name must be at least 3 characters').max(100),
  description: z.string().max(500).optional(),
  category: z.string().min(1, 'Please select a category'),
  tags: z.array(z.string()).optional(),
  metadata: z.record(z.any()).optional()
})

const templateSelectionSchema = z.object({
  useTemplate: z.boolean(),
  templateId: z.string().optional(),
  templateParameters: z.record(z.any()).optional()
})

const parameterConfigSchema = z.object({
  inputParameters: z.array(z.string()).optional(),
  outputParameters: z.array(z.string()).optional(),
  timeoutSeconds: z.number().min(0).optional(),
  retryPolicy: z.enum(['FIXED', 'EXPONENTIAL_BACKOFF']).optional(),
  maxRetries: z.number().min(0).max(10).optional()
})

const fullFormSchema = z.object({
  ...basicInfoSchema.shape,
  ...templateSelectionSchema.shape,
  ...parameterConfigSchema.shape
})

type WorkflowFormData = z.infer<typeof fullFormSchema> & {
  templateTasks?: any[]
}

const steps = [
  { id: 'basic', title: 'Basic Information', icon: FileText },
  { id: 'template', title: 'Template Selection', icon: FileText },
  { id: 'parameters', title: 'Parameters', icon: Settings },
  { id: 'review', title: 'Review & Create', icon: Eye }
]

export function WorkflowCreationWizard({
  onSubmit,
  onCancel,
  initialTemplate,
  initialTemplateParameters
}: WorkflowCreationWizardProps) {
  const [currentStep, setCurrentStep] = useState(initialTemplate ? 2 : 0)
  const [selectedTemplate, setSelectedTemplate] =
    useState<WorkflowTemplate | null>(initialTemplate || null)

  const form = useForm<WorkflowFormData>({
    resolver: zodResolver(fullFormSchema),
    defaultValues: {
      name: initialTemplate
        ? `${initialTemplate.name} - ${new Date().toLocaleDateString()}`
        : '',
      description: initialTemplate?.description || '',
      category: initialTemplate?.category || '',
      tags: initialTemplate?.tags || [],
      metadata: {},
      useTemplate: !!initialTemplate,
      templateId: initialTemplate?.id || '',
      templateParameters: initialTemplateParameters || {},
      inputParameters: initialTemplate?.workflow.inputParameters || [],
      outputParameters: initialTemplate?.workflow.outputParameters || [],
      timeoutSeconds: initialTemplate?.workflow.timeoutSeconds || 3600,
      retryPolicy: 'FIXED',
      maxRetries: 3
    }
  })

  const validateCurrentStep = async () => {
    let fieldsToValidate: (keyof WorkflowFormData)[] = []

    switch (currentStep) {
      case 0:
        fieldsToValidate = ['name', 'description', 'category']
        break
      case 1:
        if (form.getValues('useTemplate')) {
          fieldsToValidate = ['templateId']
        }
        break
      case 2:
        fieldsToValidate = [
          'inputParameters',
          'outputParameters',
          'timeoutSeconds'
        ]
        break
    }

    const result = await form.trigger(fieldsToValidate)
    return result
  }

  const handleNext = async () => {
    const isValid = await validateCurrentStep()
    if (isValid && currentStep < steps.length - 1) {
      setCurrentStep(currentStep + 1)
    }
  }

  const handlePrevious = () => {
    if (currentStep > 0) {
      setCurrentStep(currentStep - 1)
    }
  }

  const handleTemplateSelect = (
    template: WorkflowTemplate,
    parameters: Record<string, any>
  ) => {
    setSelectedTemplate(template)
    form.setValue('useTemplate', true)
    form.setValue('templateId', template.id)
    form.setValue('templateParameters', parameters)
    // Pre-fill some fields from template
    form.setValue(
      'name',
      `${template.name} - ${new Date().toLocaleDateString()}`
    )
    form.setValue('description', template.description)
    form.setValue('category', template.category)
    form.setValue('tags', template.tags)
    // Pre-fill parameters from template
    form.setValue('inputParameters', template.workflow.inputParameters || [])
    form.setValue('outputParameters', template.workflow.outputParameters || [])
    form.setValue('timeoutSeconds', template.workflow.timeoutSeconds || 3600)
  }

  const handleSubmit = form.handleSubmit((data) => {
    onSubmit({
      ...data,
      // Include template task structure if using template
      templateTasks: selectedTemplate?.workflow.tasks
    })
  })

  const progressPercentage = ((currentStep + 1) / steps.length) * 100

  return (
    <div className="space-y-4 sm:space-y-6">
      {/* Progress indicator */}
      <div className="space-y-2">
        <div className="flex justify-between text-xs text-muted-foreground sm:text-sm">
          <span>
            Step {currentStep + 1} of {steps.length}
          </span>
          <span>{progressPercentage}% Complete</span>
        </div>
        <Progress value={progressPercentage} className="h-1.5 sm:h-2" />
        <div className="mt-4 flex justify-between">
          {steps.map((step, index) => {
            const Icon = step.icon
            return (
              <div
                key={step.id}
                className={cn(
                  'flex flex-col items-center space-y-1 sm:flex-row sm:space-x-2 sm:space-y-0',
                  index <= currentStep
                    ? 'text-primary'
                    : 'text-muted-foreground'
                )}
              >
                <div
                  className={cn(
                    'flex h-6 w-6 items-center justify-center rounded-full border-2 sm:h-8 sm:w-8',
                    index < currentStep
                      ? 'border-primary bg-primary text-primary-foreground'
                      : index === currentStep
                        ? 'border-primary'
                        : 'border-muted-foreground'
                  )}
                >
                  {index < currentStep ? (
                    <Check className="h-3 w-3 sm:h-4 sm:w-4" />
                  ) : (
                    <Icon className="h-3 w-3 sm:h-4 sm:w-4" />
                  )}
                </div>
                <span className="hidden text-xs sm:inline sm:text-sm">
                  {step.title}
                </span>
              </div>
            )
          })}
        </div>
      </div>

      {/* Form content */}
      <Form {...form}>
        <form onSubmit={handleSubmit}>
          <Card>
            <CardHeader>
              <CardTitle>{steps[currentStep].title}</CardTitle>
              <CardDescription>
                {currentStep === 0 &&
                  'Enter the basic information for your workflow'}
                {currentStep === 1 &&
                  'Choose to start from a template or create from scratch'}
                {currentStep === 2 &&
                  'Configure workflow parameters and behavior'}
                {currentStep === 3 &&
                  'Review your workflow configuration before creating'}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Step 1: Basic Information */}
              {currentStep === 0 && <BasicInfoStep form={form} />}

              {/* Step 2: Template Selection */}
              {currentStep === 1 && (
                <TemplateSelectionStep
                  form={form}
                  onTemplateSelect={handleTemplateSelect}
                  selectedTemplate={selectedTemplate}
                />
              )}

              {/* Step 3: Parameter Configuration */}
              {currentStep === 2 && <ParameterConfigStep form={form} />}

              {/* Step 4: Review */}
              {currentStep === 3 && (
                <ReviewStep form={form} selectedTemplate={selectedTemplate} />
              )}
            </CardContent>
          </Card>

          {/* Navigation buttons */}
          <div className="mt-4 flex flex-col gap-2 sm:mt-6 sm:flex-row sm:justify-between">
            <div className="order-2 sm:order-1">
              {currentStep > 0 && (
                <Button
                  type="button"
                  variant="outline"
                  onClick={handlePrevious}
                  size="sm"
                  className="sm:size-default w-full sm:w-auto"
                >
                  <ChevronLeft className="mr-2 h-4 w-4" />
                  Previous
                </Button>
              )}
            </div>
            <div className="order-1 flex gap-2 sm:order-2 sm:space-x-2">
              <Button
                type="button"
                variant="outline"
                onClick={onCancel}
                size="sm"
                className="sm:size-default flex-1 sm:flex-none"
              >
                Cancel
              </Button>
              {currentStep < steps.length - 1 ? (
                <Button
                  type="button"
                  onClick={handleNext}
                  size="sm"
                  className="sm:size-default flex-1 sm:flex-none"
                >
                  Next
                  <ChevronRight className="ml-2 h-4 w-4" />
                </Button>
              ) : (
                <Button
                  type="submit"
                  size="sm"
                  className="sm:size-default flex-1 sm:flex-none"
                >
                  Create Workflow
                  <Check className="ml-2 h-4 w-4" />
                </Button>
              )}
            </div>
          </div>
        </form>
      </Form>
    </div>
  )
}

// Step Components
function BasicInfoStep({ form }: { form: UseFormReturn<WorkflowFormData> }) {
  return (
    <div className="space-y-4">
      <InputField
        control={form.control}
        name="name"
        label="Workflow Name"
        placeholder="Enter workflow name"
        description="A unique name for your workflow"
        required
      />

      <FormField
        control={form.control}
        name="description"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Description</FormLabel>
            <FormControl>
              <Textarea
                placeholder="Describe what this workflow does..."
                className="resize-none"
                {...field}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <SelectField
        control={form.control}
        name="category"
        label="Category"
        placeholder="Select a category"
        required
      >
        <option value="payments">Payments</option>
        <option value="onboarding">Onboarding</option>
        <option value="compliance">Compliance</option>
        <option value="reconciliation">Reconciliation</option>
        <option value="reporting">Reporting</option>
        <option value="notifications">Notifications</option>
        <option value="integration">Integration</option>
        <option value="custom">Custom</option>
      </SelectField>

      <FormField
        control={form.control}
        name="tags"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Tags</FormLabel>
            <FormControl>
              <Input
                placeholder="Enter tags separated by commas"
                value={field.value?.join(', ') || ''}
                onChange={(e) => {
                  const tags = e.target.value
                    .split(',')
                    .map((tag) => tag.trim())
                    .filter((tag) => tag.length > 0)
                  field.onChange(tags)
                }}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <MetadataField
        control={form.control}
        name="metadata"
        label="Additional Metadata"
        description="Add custom key-value pairs"
      />
    </div>
  )
}

function TemplateSelectionStep({
  form,
  onTemplateSelect,
  selectedTemplate
}: {
  form: UseFormReturn<WorkflowFormData>
  onTemplateSelect: (
    template: WorkflowTemplate,
    parameters: Record<string, any>
  ) => void
  selectedTemplate: WorkflowTemplate | null
}) {
  const useTemplate = form.watch('useTemplate')

  return (
    <div className="space-y-4">
      <Alert>
        <InfoIcon className="h-4 w-4" />
        <AlertDescription>
          You can start with a pre-built template or create a workflow from
          scratch. Templates provide a great starting point that you can
          customize later.
        </AlertDescription>
      </Alert>

      <Tabs
        defaultValue={useTemplate ? 'template' : 'scratch'}
        className="w-full"
      >
        <TabsList className="grid w-full grid-cols-2">
          <TabsTrigger
            value="scratch"
            onClick={() => {
              form.setValue('useTemplate', false)
              form.setValue('templateId', '')
              form.setValue('templateParameters', {})
            }}
            className="text-xs sm:text-sm"
          >
            Start from Scratch
          </TabsTrigger>
          <TabsTrigger
            value="template"
            onClick={() => form.setValue('useTemplate', true)}
            className="text-xs sm:text-sm"
          >
            Use Template
          </TabsTrigger>
        </TabsList>

        <TabsContent value="scratch" className="mt-4">
          <Card>
            <CardContent className="pt-6">
              <div className="py-8 text-center">
                <FileText className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
                <h3 className="mb-2 text-lg font-medium">
                  Create from Scratch
                </h3>
                <p className="mx-auto max-w-sm text-sm text-muted-foreground">
                  Build your workflow step by step with full control over every
                  aspect
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="template" className="mt-4">
          {selectedTemplate && (
            <Alert className="mb-4">
              <Check className="h-4 w-4" />
              <AlertDescription>
                Selected template: <strong>{selectedTemplate.name}</strong>
              </AlertDescription>
            </Alert>
          )}
          <TemplateCatalog onUseTemplate={onTemplateSelect} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

function ParameterConfigStep({
  form
}: {
  form: UseFormReturn<WorkflowFormData>
}) {
  const inputParams = form.watch('inputParameters') || []
  const outputParams = form.watch('outputParameters') || []

  const addParameter = (type: 'input' | 'output', value: string) => {
    if (!value.trim()) return

    const fieldName = type === 'input' ? 'inputParameters' : 'outputParameters'
    const currentParams = form.getValues(fieldName) || []
    if (!currentParams.includes(value)) {
      form.setValue(fieldName, [...currentParams, value])
    }
  }

  const removeParameter = (type: 'input' | 'output', value: string) => {
    const fieldName = type === 'input' ? 'inputParameters' : 'outputParameters'
    const currentParams = form.getValues(fieldName) || []
    form.setValue(
      fieldName,
      currentParams.filter((p) => p !== value)
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h3 className="mb-4 text-sm font-medium">Workflow Parameters</h3>

        {/* Input Parameters */}
        <div className="space-y-4">
          <div>
            <FormLabel>Input Parameters</FormLabel>
            <div className="mt-2 flex gap-2">
              <Input
                placeholder="Add input parameter"
                onKeyPress={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    addParameter('input', (e.target as HTMLInputElement).value)
                    ;(e.target as HTMLInputElement).value = ''
                  }
                }}
              />
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  const input = document.querySelector(
                    'input[placeholder="Add input parameter"]'
                  ) as HTMLInputElement
                  if (input) {
                    addParameter('input', input.value)
                    input.value = ''
                  }
                }}
              >
                Add
              </Button>
            </div>
            <div className="mt-2 flex flex-wrap gap-2">
              {inputParams.map((param) => (
                <Badge
                  key={param}
                  variant="secondary"
                  className="cursor-pointer"
                  onClick={() => removeParameter('input', param)}
                >
                  {param} ×
                </Badge>
              ))}
            </div>
          </div>

          {/* Output Parameters */}
          <div>
            <FormLabel>Output Parameters</FormLabel>
            <div className="mt-2 flex gap-2">
              <Input
                placeholder="Add output parameter"
                onKeyPress={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    addParameter('output', (e.target as HTMLInputElement).value)
                    ;(e.target as HTMLInputElement).value = ''
                  }
                }}
              />
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  const input = document.querySelector(
                    'input[placeholder="Add output parameter"]'
                  ) as HTMLInputElement
                  if (input) {
                    addParameter('output', input.value)
                    input.value = ''
                  }
                }}
              >
                Add
              </Button>
            </div>
            <div className="mt-2 flex flex-wrap gap-2">
              {outputParams.map((param) => (
                <Badge
                  key={param}
                  variant="secondary"
                  className="cursor-pointer"
                  onClick={() => removeParameter('output', param)}
                >
                  {param} ×
                </Badge>
              ))}
            </div>
          </div>
        </div>
      </div>

      <div>
        <h3 className="mb-4 text-sm font-medium">Execution Settings</h3>
        <div className="space-y-4">
          <InputField
            control={form.control}
            name="timeoutSeconds"
            label="Timeout (seconds)"
            type="number"
            placeholder="3600"
            description="Maximum time allowed for workflow execution"
          />

          <SelectField
            control={form.control}
            name="retryPolicy"
            label="Retry Policy"
            placeholder="Select retry policy"
          >
            <option value="FIXED">Fixed Delay</option>
            <option value="EXPONENTIAL_BACKOFF">Exponential Backoff</option>
          </SelectField>

          <InputField
            control={form.control}
            name="maxRetries"
            label="Max Retries"
            type="number"
            placeholder="3"
            description="Maximum number of retry attempts"
          />
        </div>
      </div>
    </div>
  )
}

function ReviewStep({
  form,
  selectedTemplate
}: {
  form: UseFormReturn<WorkflowFormData>
  selectedTemplate: WorkflowTemplate | null
}) {
  const formData = form.getValues()

  return (
    <div className="space-y-6">
      <Alert>
        <InfoIcon className="h-4 w-4" />
        <AlertDescription>
          Please review your workflow configuration before creating. You can
          edit these settings later.
        </AlertDescription>
      </Alert>

      <div className="space-y-4">
        <div className="space-y-3 rounded-lg border p-3 sm:p-4">
          <h3 className="text-sm font-medium sm:text-base">
            Basic Information
          </h3>
          <div className="grid grid-cols-1 gap-3 text-xs sm:grid-cols-2 sm:gap-4 sm:text-sm">
            <div>
              <span className="text-muted-foreground">Name:</span>
              <p className="font-medium">{formData.name || 'Not set'}</p>
            </div>
            <div>
              <span className="text-muted-foreground">Category:</span>
              <p className="font-medium capitalize">
                {formData.category || 'Not set'}
              </p>
            </div>
            <div className="col-span-1 sm:col-span-2">
              <span className="text-muted-foreground">Description:</span>
              <p className="font-medium">
                {formData.description || 'Not provided'}
              </p>
            </div>
            {formData.tags && formData.tags.length > 0 && (
              <div className="col-span-1 sm:col-span-2">
                <span className="text-muted-foreground">Tags:</span>
                <div className="mt-1 flex flex-wrap gap-1">
                  {formData.tags.map((tag) => (
                    <Badge
                      key={tag}
                      variant="secondary"
                      className="text-[10px] sm:text-xs"
                    >
                      {tag}
                    </Badge>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>

        {formData.useTemplate && selectedTemplate && (
          <div className="space-y-3 rounded-lg border p-3 sm:p-4">
            <h3 className="text-sm font-medium sm:text-base">Template</h3>
            <div className="text-xs sm:text-sm">
              <span className="text-muted-foreground">Using template:</span>
              <p className="font-medium">{selectedTemplate.name}</p>
            </div>
          </div>
        )}

        <div className="space-y-3 rounded-lg border p-3 sm:p-4">
          <h3 className="text-sm font-medium sm:text-base">Parameters</h3>
          <div className="grid grid-cols-1 gap-3 text-xs sm:grid-cols-2 sm:gap-4 sm:text-sm">
            <div>
              <span className="text-muted-foreground">Input Parameters:</span>
              <p className="font-medium">
                {formData.inputParameters?.length || 0} parameters
              </p>
            </div>
            <div>
              <span className="text-muted-foreground">Output Parameters:</span>
              <p className="font-medium">
                {formData.outputParameters?.length || 0} parameters
              </p>
            </div>
          </div>
        </div>

        <div className="space-y-3 rounded-lg border p-3 sm:p-4">
          <h3 className="text-sm font-medium sm:text-base">
            Execution Settings
          </h3>
          <div className="grid grid-cols-1 gap-3 text-xs sm:grid-cols-2 sm:gap-4 sm:text-sm">
            <div>
              <span className="text-muted-foreground">Timeout:</span>
              <p className="font-medium">{formData.timeoutSeconds} seconds</p>
            </div>
            <div>
              <span className="text-muted-foreground">Retry Policy:</span>
              <p className="font-medium">
                {formData.retryPolicy?.replace('_', ' ')}
              </p>
            </div>
            <div>
              <span className="text-muted-foreground">Max Retries:</span>
              <p className="font-medium">{formData.maxRetries}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// Add missing import
import { Input } from '@/components/ui/input'
