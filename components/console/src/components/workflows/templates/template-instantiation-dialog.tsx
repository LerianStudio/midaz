'use client'

import { useState } from 'react'
import {
  WorkflowTemplate,
  TemplateParameter
} from '@/core/domain/entities/workflow-template'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
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
import { Separator } from '@/components/ui/separator'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Play, AlertTriangle, Info, CheckCircle } from 'lucide-react'

interface TemplateInstantiationDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  template: WorkflowTemplate
  onInstantiate: (parameters: Record<string, any>) => void
}

export function TemplateInstantiationDialog({
  open,
  onOpenChange,
  template,
  onInstantiate
}: TemplateInstantiationDialogProps) {
  const [workflowName, setWorkflowName] = useState(
    `${template.name} - ${new Date().toLocaleDateString()}`
  )
  const [workflowDescription, setWorkflowDescription] = useState('')
  const [parameters, setParameters] = useState<Record<string, any>>({})
  const [validationErrors, setValidationErrors] = useState<
    Record<string, string>
  >({})
  const [isValidating, setIsValidating] = useState(false)

  const handleParameterChange = (paramName: string, value: any) => {
    setParameters((prev) => ({
      ...prev,
      [paramName]: value
    }))

    // Clear validation error for this parameter
    if (validationErrors[paramName]) {
      setValidationErrors((prev) => {
        const newErrors = { ...prev }
        delete newErrors[paramName]
        return newErrors
      })
    }
  }

  const validateParameter = (
    param: TemplateParameter,
    value: any
  ): string | null => {
    if (
      param.required &&
      (value === undefined || value === null || value === '')
    ) {
      return `${param.name} is required`
    }

    if (value && param.validation) {
      const validation = param.validation

      if (validation.pattern && typeof value === 'string') {
        const regex = new RegExp(validation.pattern)
        if (!regex.test(value)) {
          return `${param.name} format is invalid`
        }
      }

      if (
        validation.minLength &&
        typeof value === 'string' &&
        value.length < validation.minLength
      ) {
        return `${param.name} must be at least ${validation.minLength} characters`
      }

      if (
        validation.maxLength &&
        typeof value === 'string' &&
        value.length > validation.maxLength
      ) {
        return `${param.name} must be at most ${validation.maxLength} characters`
      }

      if (
        validation.min &&
        typeof value === 'number' &&
        value < validation.min
      ) {
        return `${param.name} must be at least ${validation.min}`
      }

      if (
        validation.max &&
        typeof value === 'number' &&
        value > validation.max
      ) {
        return `${param.name} must be at most ${validation.max}`
      }
    }

    return null
  }

  const validateAllParameters = (): boolean => {
    const errors: Record<string, string> = {}

    template.parameters.forEach((param) => {
      const value = parameters[param.name] ?? param.defaultValue
      const error = validateParameter(param, value)
      if (error) {
        errors[param.name] = error
      }
    })

    setValidationErrors(errors)
    return Object.keys(errors).length === 0
  }

  const handleInstantiate = async () => {
    setIsValidating(true)

    if (!workflowName.trim()) {
      setValidationErrors((prev) => ({
        ...prev,
        workflowName: 'Workflow name is required'
      }))
      setIsValidating(false)
      return
    }

    if (!validateAllParameters()) {
      setIsValidating(false)
      return
    }

    // Apply default values for parameters not set
    const finalParameters = { ...parameters }
    template.parameters.forEach((param) => {
      if (
        finalParameters[param.name] === undefined &&
        param.defaultValue !== undefined
      ) {
        finalParameters[param.name] = param.defaultValue
      }
    })

    const instantiationData = {
      workflowName: workflowName.trim(),
      description: workflowDescription.trim(),
      templateId: template.id,
      parameters: finalParameters
    }

    try {
      onInstantiate(instantiationData)
    } catch (error) {
      console.error('Failed to instantiate template:', error)
    } finally {
      setIsValidating(false)
    }
  }

  const renderParameterInput = (param: TemplateParameter) => {
    const value = parameters[param.name] ?? param.defaultValue ?? ''
    const hasError = !!validationErrors[param.name]

    switch (param.type) {
      case 'string':
        return (
          <Input
            value={value}
            onChange={(e) => handleParameterChange(param.name, e.target.value)}
            placeholder={`Enter ${param.name}`}
            className={hasError ? 'border-red-500' : ''}
          />
        )

      case 'number':
        return (
          <Input
            type="number"
            value={value}
            onChange={(e) =>
              handleParameterChange(param.name, parseFloat(e.target.value) || 0)
            }
            placeholder={`Enter ${param.name}`}
            className={hasError ? 'border-red-500' : ''}
          />
        )

      case 'boolean':
        return (
          <div className="flex items-center space-x-2">
            <Checkbox
              checked={!!value}
              onCheckedChange={(checked) =>
                handleParameterChange(param.name, checked)
              }
            />
            <Label className="text-sm">Enable {param.name}</Label>
          </div>
        )

      case 'select':
        return (
          <Select
            value={value}
            onValueChange={(newValue) =>
              handleParameterChange(param.name, newValue)
            }
          >
            <SelectTrigger className={hasError ? 'border-red-500' : ''}>
              <SelectValue placeholder={`Select ${param.name}`} />
            </SelectTrigger>
            <SelectContent>
              {param.options?.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  <div>
                    <div>{option.label}</div>
                    {option.description && (
                      <div className="text-xs text-muted-foreground">
                        {option.description}
                      </div>
                    )}
                  </div>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )

      case 'multiselect':
        return (
          <div className="space-y-2">
            {param.options?.map((option) => (
              <div key={option.value} className="flex items-center space-x-2">
                <Checkbox
                  checked={Array.isArray(value) && value.includes(option.value)}
                  onCheckedChange={(checked) => {
                    const currentValues = Array.isArray(value) ? value : []
                    const newValues = checked
                      ? [...currentValues, option.value]
                      : currentValues.filter((v) => v !== option.value)
                    handleParameterChange(param.name, newValues)
                  }}
                />
                <Label className="text-sm">{option.label}</Label>
              </div>
            ))}
          </div>
        )

      case 'object':
        return (
          <Textarea
            value={
              typeof value === 'string' ? value : JSON.stringify(value, null, 2)
            }
            onChange={(e) => {
              try {
                const parsed = JSON.parse(e.target.value)
                handleParameterChange(param.name, parsed)
              } catch {
                handleParameterChange(param.name, e.target.value)
              }
            }}
            placeholder={`Enter ${param.name} as JSON`}
            rows={4}
            className={hasError ? 'border-red-500' : ''}
          />
        )

      default:
        return (
          <Input
            value={value}
            onChange={(e) => handleParameterChange(param.name, e.target.value)}
            placeholder={`Enter ${param.name}`}
            className={hasError ? 'border-red-500' : ''}
          />
        )
    }
  }

  const getParameterStatus = (param: TemplateParameter) => {
    const value = parameters[param.name]
    const hasValue = value !== undefined && value !== null && value !== ''
    const hasDefault = param.defaultValue !== undefined

    if (param.required) {
      if (hasValue || hasDefault) {
        return <CheckCircle className="h-4 w-4 text-green-600" />
      } else {
        return <AlertTriangle className="h-4 w-4 text-red-600" />
      }
    } else {
      return <Info className="h-4 w-4 text-blue-600" />
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Create Workflow from Template</DialogTitle>
          <DialogDescription>
            Configure parameters to create a new workflow from &quot;
            {template.name}&quot;
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6">
          {/* Workflow Basic Info */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium">
                Workflow Details
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div>
                <Label htmlFor="workflowName">Workflow Name *</Label>
                <Input
                  id="workflowName"
                  value={workflowName}
                  onChange={(e) => setWorkflowName(e.target.value)}
                  placeholder="Enter workflow name"
                  className={
                    validationErrors.workflowName ? 'border-red-500' : ''
                  }
                />
                {validationErrors.workflowName && (
                  <p className="mt-1 text-xs text-red-600">
                    {validationErrors.workflowName}
                  </p>
                )}
              </div>

              <div>
                <Label htmlFor="workflowDescription">
                  Description (Optional)
                </Label>
                <Textarea
                  id="workflowDescription"
                  value={workflowDescription}
                  onChange={(e) => setWorkflowDescription(e.target.value)}
                  placeholder="Enter workflow description"
                  rows={2}
                />
              </div>
            </CardContent>
          </Card>

          {/* Template Parameters */}
          {template.parameters.length > 0 && (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-medium">
                  Template Parameters
                </CardTitle>
                <CardDescription>
                  Configure the parameters required by this template
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                {template.parameters.map((param, index) => (
                  <div key={param.name}>
                    <div className="mb-2 flex items-center gap-2">
                      {getParameterStatus(param)}
                      <Label htmlFor={param.name} className="font-medium">
                        {param.name}
                      </Label>
                      <Badge variant="outline" className="text-xs">
                        {param.type}
                      </Badge>
                      {param.required && (
                        <Badge variant="destructive" className="text-xs">
                          Required
                        </Badge>
                      )}
                    </div>

                    <p className="mb-3 text-sm text-muted-foreground">
                      {param.description}
                    </p>

                    {renderParameterInput(param)}

                    {validationErrors[param.name] && (
                      <p className="mt-1 text-xs text-red-600">
                        {validationErrors[param.name]}
                      </p>
                    )}

                    {param.defaultValue !== undefined &&
                      !parameters[param.name] && (
                        <p className="mt-1 text-xs text-muted-foreground">
                          Default: {JSON.stringify(param.defaultValue)}
                        </p>
                      )}

                    {index < template.parameters.length - 1 && (
                      <Separator className="mt-4" />
                    )}
                  </div>
                ))}
              </CardContent>
            </Card>
          )}

          {/* Validation Summary */}
          {Object.keys(validationErrors).length > 0 && (
            <Alert variant="destructive">
              <AlertTriangle className="h-4 w-4" />
              <AlertDescription>
                Please fix the validation errors above before proceeding.
              </AlertDescription>
            </Alert>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleInstantiate}
            disabled={isValidating}
            className="min-w-[120px]"
          >
            {isValidating ? (
              'Creating...'
            ) : (
              <>
                <Play className="mr-2 h-4 w-4" />
                Create Workflow
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
