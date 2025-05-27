'use client'

import React, { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
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
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Progress } from '@/components/ui/progress'
import {
  ArrowLeft,
  ArrowRight,
  Check,
  FileText,
  Settings,
  Database,
  Eye,
  Upload
} from 'lucide-react'
import { useToast } from '@/hooks/use-toast'
import { TemplateFileUpload } from './template-file-upload'
import type {
  TemplateCategory,
  TemplateFormat,
  CreateTemplateInput
} from '@/core/domain/entities/template'

interface WizardStep {
  id: string
  title: string
  description: string
  icon: React.ReactNode
}

const steps: WizardStep[] = [
  {
    id: 'basic',
    title: 'Basic Information',
    description: 'Template name, description, and category',
    icon: <FileText className="h-4 w-4" />
  },
  {
    id: 'configuration',
    title: 'Configuration',
    description: 'Format, engine, and output settings',
    icon: <Settings className="h-4 w-4" />
  },
  {
    id: 'datasources',
    title: 'Data Sources',
    description: 'Connect template to data sources',
    icon: <Database className="h-4 w-4" />
  },
  {
    id: 'preview',
    title: 'Preview & Create',
    description: 'Review and create your template',
    icon: <Eye className="h-4 w-4" />
  }
]

const categories: TemplateCategory[] = [
  'FINANCIAL',
  'OPERATIONAL',
  'COMPLIANCE',
  'MARKETING',
  'CUSTOM'
]
const formats = ['PDF', 'HTML', 'EXCEL', 'WORD', 'CSV'] as const
type TemplateFormat = (typeof formats)[number]

interface ExtendedTemplateInput extends Partial<CreateTemplateInput> {
  format?: TemplateFormat
  engine?: string
  dataSourceIds?: string[]
  variables?: string[]
  metadata?: Record<string, any>
}

export function TemplateCreationWizard() {
  const router = useRouter()
  const [currentStep, setCurrentStep] = useState(0)
  const [formData, setFormData] = useState<ExtendedTemplateInput>({
    name: '',
    description: '',
    category: undefined,
    format: undefined,
    engine: 'pongo2',
    dataSourceIds: [],
    variables: [],
    metadata: {}
  })

  const progress = ((currentStep + 1) / steps.length) * 100

  const updateFormData = (updates: Partial<typeof formData>) => {
    setFormData((prev) => ({ ...prev, ...updates }))
  }

  const nextStep = () => {
    if (currentStep < steps.length - 1) {
      setCurrentStep(currentStep + 1)
    }
  }

  const prevStep = () => {
    if (currentStep > 0) {
      setCurrentStep(currentStep - 1)
    }
  }

  const handleSubmit = () => {
    console.log('Creating template:', formData)
    // Here we would call the API to create the template
    router.push('/plugins/smart-templates/templates')
  }

  const canProceed = () => {
    switch (currentStep) {
      case 0:
        return formData.name && formData.description && formData.category
      case 1:
        return formData.format
      case 2:
        return true // Data sources are optional
      case 3:
        return true
      default:
        return false
    }
  }

  const renderStepContent = () => {
    switch (currentStep) {
      case 0:
        return (
          <div className="space-y-6">
            <div className="space-y-2">
              <Label htmlFor="name">Template Name *</Label>
              <Input
                id="name"
                placeholder="e.g., Monthly Financial Report"
                value={formData.name || ''}
                onChange={(e) => updateFormData({ name: e.target.value })}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="description">Description *</Label>
              <Textarea
                id="description"
                placeholder="Describe what this template is used for..."
                value={formData.description || ''}
                onChange={(e) =>
                  updateFormData({ description: e.target.value })
                }
                rows={3}
              />
            </div>

            <div className="space-y-2">
              <Label>Category *</Label>
              <Select
                value={formData.category}
                onValueChange={(value: TemplateCategory) =>
                  updateFormData({ category: value })
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select a category" />
                </SelectTrigger>
                <SelectContent>
                  {categories.map((category) => (
                    <SelectItem key={category} value={category}>
                      {category}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="tags">Tags (Optional)</Label>
              <Input
                id="tags"
                placeholder="Enter tags separated by commas"
                onChange={(e) => {
                  const tags = e.target.value
                    .split(',')
                    .map((tag) => tag.trim())
                    .filter(Boolean)
                  updateFormData({ metadata: { ...formData.metadata, tags } })
                }}
              />
              <p className="text-sm text-muted-foreground">
                Tags help organize and find templates later
              </p>
            </div>
          </div>
        )

      case 1:
        return (
          <div className="space-y-6">
            <div className="space-y-2">
              <Label>Output Format *</Label>
              <Select
                value={formData.format}
                onValueChange={(value: TemplateFormat) =>
                  updateFormData({ format: value })
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select output format" />
                </SelectTrigger>
                <SelectContent>
                  {formats.map((format) => (
                    <SelectItem key={format} value={format}>
                      {format}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Template Engine</Label>
              <Select
                value={formData.engine}
                onValueChange={(value) => updateFormData({ engine: value })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="pongo2">Pongo2 (Recommended)</SelectItem>
                  <SelectItem value="go-template">Go Template</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-sm text-muted-foreground">
                Pongo2 offers more features and better Django/Jinja2
                compatibility
              </p>
            </div>

            <div className="space-y-4">
              <Label>Template Options</Label>
              <div className="space-y-3">
                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="auto-generation"
                    checked={formData.metadata?.autoGeneration || false}
                    onCheckedChange={(checked) =>
                      updateFormData({
                        metadata: {
                          ...formData.metadata,
                          autoGeneration: checked as boolean
                        }
                      })
                    }
                  />
                  <Label htmlFor="auto-generation" className="text-sm">
                    Enable automatic report generation
                  </Label>
                </div>

                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="version-control"
                    checked={formData.metadata?.versionControl !== false}
                    onCheckedChange={(checked) =>
                      updateFormData({
                        metadata: {
                          ...formData.metadata,
                          versionControl: checked as boolean
                        }
                      })
                    }
                  />
                  <Label htmlFor="version-control" className="text-sm">
                    Enable version control
                  </Label>
                </div>
              </div>
            </div>
          </div>
        )

      case 2:
        return (
          <div className="space-y-6">
            <div>
              <h3 className="mb-2 text-lg font-medium">
                Available Data Sources
              </h3>
              <p className="mb-4 text-sm text-muted-foreground">
                Select the data sources this template will use to generate
                reports
              </p>
            </div>

            <div className="space-y-3">
              {[
                {
                  id: 'ds-1',
                  name: 'Midaz Core API',
                  description: 'Transaction and account data',
                  type: 'API'
                },
                {
                  id: 'ds-2',
                  name: 'Analytics Database',
                  description: 'Aggregated financial metrics',
                  type: 'Database'
                },
                {
                  id: 'ds-3',
                  name: 'External Bank APIs',
                  description: 'Real-time balance updates',
                  type: 'API'
                },
                {
                  id: 'ds-4',
                  name: 'User Management',
                  description: 'Customer and user information',
                  type: 'Service'
                }
              ].map((source) => (
                <Card
                  key={source.id}
                  className="cursor-pointer hover:bg-accent/50"
                >
                  <CardContent className="p-4">
                    <div className="flex items-center space-x-3">
                      <Checkbox
                        checked={
                          formData.dataSourceIds?.includes(source.id) || false
                        }
                        onCheckedChange={(checked) => {
                          const currentIds = formData.dataSourceIds || []
                          const newIds = checked
                            ? [...currentIds, source.id]
                            : currentIds.filter((id) => id !== source.id)
                          updateFormData({ dataSourceIds: newIds })
                        }}
                      />
                      <div className="flex-1">
                        <div className="flex items-center space-x-2">
                          <h4 className="font-medium">{source.name}</h4>
                          <Badge variant="secondary" className="text-xs">
                            {source.type}
                          </Badge>
                        </div>
                        <p className="text-sm text-muted-foreground">
                          {source.description}
                        </p>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>

            <div className="rounded-lg bg-blue-50 p-4 dark:bg-blue-900/20">
              <h4 className="mb-1 font-medium text-blue-900 dark:text-blue-100">
                💡 Pro Tip
              </h4>
              <p className="text-sm text-blue-800 dark:text-blue-200">
                You can add more data sources later or create custom ones in the
                Data Sources section.
              </p>
            </div>
          </div>
        )

      case 3:
        return (
          <div className="space-y-6">
            <div>
              <h3 className="mb-2 text-lg font-medium">
                Review Template Configuration
              </h3>
              <p className="text-sm text-muted-foreground">
                Please review your template settings before creating
              </p>
            </div>

            <div className="space-y-4">
              <Card>
                <CardHeader className="pb-3">
                  <CardTitle className="text-base">Basic Information</CardTitle>
                </CardHeader>
                <CardContent className="space-y-2">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Name:</span>
                    <span className="font-medium">{formData.name}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Category:</span>
                    <Badge variant="secondary">{formData.category}</Badge>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Description:</span>
                    <span className="max-w-xs text-right">
                      {formData.description}
                    </span>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="pb-3">
                  <CardTitle className="text-base">Configuration</CardTitle>
                </CardHeader>
                <CardContent className="space-y-2">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Format:</span>
                    <Badge>{formData.format}</Badge>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Engine:</span>
                    <span className="font-medium">{formData.engine}</span>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="pb-3">
                  <CardTitle className="text-base">Data Sources</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-muted-foreground">
                    {formData.dataSourceIds?.length || 0} data sources selected
                  </div>
                </CardContent>
              </Card>
            </div>

            <div className="rounded-lg bg-green-50 p-4 dark:bg-green-900/20">
              <h4 className="mb-1 font-medium text-green-900 dark:text-green-100">
                ✅ Ready to Create
              </h4>
              <p className="text-sm text-green-800 dark:text-green-200">
                Your template will be created with a basic structure. You can
                customize the content using the template editor after creation.
              </p>
            </div>
          </div>
        )

      default:
        return null
    }
  }

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header */}
      <div className="flex items-center space-x-4">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => router.back()}
          className="flex items-center space-x-2"
        >
          <ArrowLeft className="h-4 w-4" />
          <span>Back</span>
        </Button>
        <div>
          <h1 className="text-2xl font-bold">Create New Template</h1>
          <p className="text-muted-foreground">
            Follow the steps to create a new template for report generation
          </p>
        </div>
      </div>

      {/* Progress */}
      <div className="space-y-2">
        <div className="flex justify-between text-sm">
          <span>
            Step {currentStep + 1} of {steps.length}
          </span>
          <span>{Math.round(progress)}% complete</span>
        </div>
        <Progress value={progress} className="h-2" />
      </div>

      {/* Steps Navigation */}
      <div className="flex items-center space-x-1 overflow-x-auto pb-2">
        {steps.map((step, index) => (
          <div
            key={step.id}
            className={`flex items-center space-x-2 whitespace-nowrap rounded-lg px-3 py-2 text-sm ${
              index === currentStep
                ? 'bg-primary text-primary-foreground'
                : index < currentStep
                  ? 'bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-200'
                  : 'bg-muted text-muted-foreground'
            }`}
          >
            {index < currentStep ? <Check className="h-4 w-4" /> : step.icon}
            <span className="font-medium">{step.title}</span>
          </div>
        ))}
      </div>

      {/* Main Content */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center space-x-2">
            {steps[currentStep].icon}
            <span>{steps[currentStep].title}</span>
          </CardTitle>
          <CardDescription>{steps[currentStep].description}</CardDescription>
        </CardHeader>
        <CardContent>{renderStepContent()}</CardContent>
      </Card>

      {/* Actions */}
      <div className="flex justify-between">
        <Button
          variant="outline"
          onClick={prevStep}
          disabled={currentStep === 0}
          className="flex items-center space-x-2"
        >
          <ArrowLeft className="h-4 w-4" />
          <span>Previous</span>
        </Button>

        {currentStep === steps.length - 1 ? (
          <Button
            onClick={handleSubmit}
            disabled={!canProceed()}
            className="flex items-center space-x-2"
          >
            <Check className="h-4 w-4" />
            <span>Create Template</span>
          </Button>
        ) : (
          <Button
            onClick={nextStep}
            disabled={!canProceed()}
            className="flex items-center space-x-2"
          >
            <span>Next</span>
            <ArrowRight className="h-4 w-4" />
          </Button>
        )}
      </div>
    </div>
  )
}
