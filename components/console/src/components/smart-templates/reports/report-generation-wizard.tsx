'use client'

import React, { useState, useEffect } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
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
import { Calendar } from '@/components/ui/calendar'
import {
  Popover,
  PopoverContent,
  PopoverTrigger
} from '@/components/ui/popover'
import {
  ArrowLeft,
  ArrowRight,
  Check,
  FileText,
  Settings,
  Database,
  Eye,
  Play,
  Calendar as CalendarIcon,
  Download,
  Mail,
  Clock,
  AlertCircle
} from 'lucide-react'
import { format } from 'date-fns'
import { Template } from '@/core/domain/entities/template'
import { mockTemplates } from '@/lib/mock-data/smart-templates'

interface WizardStep {
  id: string
  title: string
  description: string
  icon: React.ReactNode
}

const steps: WizardStep[] = [
  {
    id: 'template',
    title: 'Select Template',
    description: 'Choose the template for report generation',
    icon: <FileText className="h-4 w-4" />
  },
  {
    id: 'parameters',
    title: 'Parameters',
    description: 'Configure template variables and data sources',
    icon: <Settings className="h-4 w-4" />
  },
  {
    id: 'data',
    title: 'Data Configuration',
    description: 'Set up data filters and date ranges',
    icon: <Database className="h-4 w-4" />
  },
  {
    id: 'output',
    title: 'Output & Delivery',
    description: 'Configure output settings and delivery options',
    icon: <Eye className="h-4 w-4" />
  }
]

interface ReportGenerationRequest {
  templateId: string
  name: string
  description?: string
  parameters: Record<string, any>
  dateRange: {
    start: Date | null
    end: Date | null
  }
  filters: Record<string, any>
  outputFormat?: string
  deliveryOptions: {
    downloadImmediately: boolean
    emailRecipients: string[]
    scheduleGeneration: boolean
    scheduleType?: 'once' | 'daily' | 'weekly' | 'monthly'
    scheduleDate?: Date
  }
}

export function ReportGenerationWizard() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const templateIdFromParams = searchParams.get('templateId')

  const [currentStep, setCurrentStep] = useState(0)
  const [templates] = useState<Template[]>(mockTemplates)
  const [selectedTemplate, setSelectedTemplate] = useState<Template | null>(
    templateIdFromParams
      ? templates.find((t) => t.id === templateIdFromParams) || null
      : null
  )
  const [generationRequest, setGenerationRequest] =
    useState<ReportGenerationRequest>({
      templateId: templateIdFromParams || '',
      name: '',
      description: '',
      parameters: {},
      dateRange: { start: null, end: null },
      filters: {},
      outputFormat: undefined,
      deliveryOptions: {
        downloadImmediately: true,
        emailRecipients: [],
        scheduleGeneration: false
      }
    })
  const [isGenerating, setIsGenerating] = useState(false)
  const [generationProgress, setGenerationProgress] = useState(0)

  const progress = ((currentStep + 1) / steps.length) * 100

  useEffect(() => {
    if (templateIdFromParams && selectedTemplate) {
      setGenerationRequest((prev) => ({
        ...prev,
        templateId: selectedTemplate.id,
        name: `${selectedTemplate.name} Report - ${format(new Date(), 'MMM dd, yyyy')}`,
        outputFormat: selectedTemplate.format
      }))
    }
  }, [templateIdFromParams, selectedTemplate])

  const updateGenerationRequest = (
    updates: Partial<ReportGenerationRequest>
  ) => {
    setGenerationRequest((prev) => ({ ...prev, ...updates }))
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

  const canProceed = () => {
    switch (currentStep) {
      case 0:
        return selectedTemplate !== null
      case 1:
        return generationRequest.name.trim() !== ''
      case 2:
        return true // Data configuration is optional
      case 3:
        return true
      default:
        return false
    }
  }

  const handleGenerate = async () => {
    setIsGenerating(true)
    setGenerationProgress(0)

    // Simulate report generation progress
    const progressInterval = setInterval(() => {
      setGenerationProgress((prev) => {
        if (prev >= 100) {
          clearInterval(progressInterval)
          setTimeout(() => {
            setIsGenerating(false)
            router.push('/plugins/smart-templates/reports')
          }, 1000)
          return 100
        }
        return prev + 10
      })
    }, 500)

    console.log('Generating report:', generationRequest)
  }

  const renderStepContent = () => {
    switch (currentStep) {
      case 0:
        return (
          <div className="space-y-6">
            {selectedTemplate ? (
              <Card className="border-primary">
                <CardContent className="p-4">
                  <div className="flex items-start justify-between">
                    <div className="flex-1">
                      <h3 className="mb-1 font-medium">
                        {selectedTemplate.name}
                      </h3>
                      <p className="mb-2 text-sm text-muted-foreground">
                        {selectedTemplate.description}
                      </p>
                      <div className="flex items-center space-x-2">
                        <Badge variant="secondary">
                          {selectedTemplate.category}
                        </Badge>
                        <Badge variant="outline">
                          {selectedTemplate.format}
                        </Badge>
                        <Badge
                          className={
                            selectedTemplate.status === 'active'
                              ? 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200'
                              : 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200'
                          }
                        >
                          {selectedTemplate.status}
                        </Badge>
                      </div>
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setSelectedTemplate(null)}
                    >
                      Change
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ) : (
              <div className="space-y-4">
                <h3 className="font-medium">Select a Template</h3>
                <div className="grid gap-3">
                  {templates
                    .filter((t) => t.status === 'active')
                    .map((template) => (
                      <Card
                        key={template.id}
                        className="cursor-pointer transition-shadow hover:shadow-md"
                        onClick={() => setSelectedTemplate(template)}
                      >
                        <CardContent className="p-4">
                          <div className="flex items-start justify-between">
                            <div className="flex-1">
                              <h4 className="mb-1 font-medium">
                                {template.name}
                              </h4>
                              <p className="mb-2 text-sm text-muted-foreground">
                                {template.description}
                              </p>
                              <div className="flex items-center space-x-2">
                                <Badge variant="secondary">
                                  {template.category}
                                </Badge>
                                <Badge variant="outline">
                                  {template.format}
                                </Badge>
                              </div>
                            </div>
                            <ArrowRight className="h-4 w-4 text-muted-foreground" />
                          </div>
                        </CardContent>
                      </Card>
                    ))}
                </div>
              </div>
            )}
          </div>
        )

      case 1:
        return (
          <div className="space-y-6">
            <div className="space-y-2">
              <Label htmlFor="report-name">Report Name *</Label>
              <Input
                id="report-name"
                placeholder="e.g., Monthly Financial Report - December 2024"
                value={generationRequest.name}
                onChange={(e) =>
                  updateGenerationRequest({ name: e.target.value })
                }
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="report-description">Description (Optional)</Label>
              <Textarea
                id="report-description"
                placeholder="Describe the purpose of this report..."
                value={generationRequest.description || ''}
                onChange={(e) =>
                  updateGenerationRequest({ description: e.target.value })
                }
                rows={3}
              />
            </div>

            {selectedTemplate && (
              <div className="space-y-4">
                <h4 className="font-medium">Template Parameters</h4>
                <div className="grid gap-4">
                  {[
                    {
                      key: 'title',
                      label: 'Report Title',
                      type: 'text',
                      placeholder: 'Enter report title'
                    },
                    {
                      key: 'company_name',
                      label: 'Company Name',
                      type: 'text',
                      placeholder: 'Your Company Name'
                    },
                    {
                      key: 'author',
                      label: 'Report Author',
                      type: 'text',
                      placeholder: 'Author name'
                    },
                    {
                      key: 'summary_text',
                      label: 'Summary Text',
                      type: 'textarea',
                      placeholder: 'Brief summary...'
                    }
                  ].map((param) => (
                    <div key={param.key} className="space-y-2">
                      <Label htmlFor={param.key}>{param.label}</Label>
                      {param.type === 'textarea' ? (
                        <Textarea
                          id={param.key}
                          placeholder={param.placeholder}
                          value={generationRequest.parameters[param.key] || ''}
                          onChange={(e) =>
                            updateGenerationRequest({
                              parameters: {
                                ...generationRequest.parameters,
                                [param.key]: e.target.value
                              }
                            })
                          }
                          rows={2}
                        />
                      ) : (
                        <Input
                          id={param.key}
                          placeholder={param.placeholder}
                          value={generationRequest.parameters[param.key] || ''}
                          onChange={(e) =>
                            updateGenerationRequest({
                              parameters: {
                                ...generationRequest.parameters,
                                [param.key]: e.target.value
                              }
                            })
                          }
                        />
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )

      case 2:
        return (
          <div className="space-y-6">
            <div className="space-y-4">
              <h4 className="font-medium">Date Range</h4>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>Start Date</Label>
                  <Popover>
                    <PopoverTrigger asChild>
                      <Button
                        variant="outline"
                        className="w-full justify-start text-left"
                      >
                        <CalendarIcon className="mr-2 h-4 w-4" />
                        {generationRequest.dateRange.start ? (
                          format(generationRequest.dateRange.start, 'PPP')
                        ) : (
                          <span>Pick a date</span>
                        )}
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent className="w-auto p-0">
                      <Calendar
                        mode="single"
                        selected={
                          generationRequest.dateRange.start || undefined
                        }
                        onSelect={(date) =>
                          updateGenerationRequest({
                            dateRange: {
                              ...generationRequest.dateRange,
                              start: date || null
                            }
                          })
                        }
                        initialFocus
                      />
                    </PopoverContent>
                  </Popover>
                </div>

                <div className="space-y-2">
                  <Label>End Date</Label>
                  <Popover>
                    <PopoverTrigger asChild>
                      <Button
                        variant="outline"
                        className="w-full justify-start text-left"
                      >
                        <CalendarIcon className="mr-2 h-4 w-4" />
                        {generationRequest.dateRange.end ? (
                          format(generationRequest.dateRange.end, 'PPP')
                        ) : (
                          <span>Pick a date</span>
                        )}
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent className="w-auto p-0">
                      <Calendar
                        mode="single"
                        selected={generationRequest.dateRange.end || undefined}
                        onSelect={(date) =>
                          updateGenerationRequest({
                            dateRange: {
                              ...generationRequest.dateRange,
                              end: date || null
                            }
                          })
                        }
                        initialFocus
                      />
                    </PopoverContent>
                  </Popover>
                </div>
              </div>
            </div>

            <div className="space-y-4">
              <h4 className="font-medium">Data Filters</h4>
              <div className="grid gap-4">
                <div className="space-y-2">
                  <Label>Account Type</Label>
                  <Select
                    value={generationRequest.filters.accountType || ''}
                    onValueChange={(value) =>
                      updateGenerationRequest({
                        filters: {
                          ...generationRequest.filters,
                          accountType: value
                        }
                      })
                    }
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="All account types" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="">All account types</SelectItem>
                      <SelectItem value="checking">Checking</SelectItem>
                      <SelectItem value="savings">Savings</SelectItem>
                      <SelectItem value="investment">Investment</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <Label>Transaction Status</Label>
                  <Select
                    value={generationRequest.filters.transactionStatus || ''}
                    onValueChange={(value) =>
                      updateGenerationRequest({
                        filters: {
                          ...generationRequest.filters,
                          transactionStatus: value
                        }
                      })
                    }
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="All statuses" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="">All statuses</SelectItem>
                      <SelectItem value="completed">Completed</SelectItem>
                      <SelectItem value="pending">Pending</SelectItem>
                      <SelectItem value="failed">Failed</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <Label>Minimum Amount</Label>
                  <Input
                    type="number"
                    placeholder="0.00"
                    value={generationRequest.filters.minAmount || ''}
                    onChange={(e) =>
                      updateGenerationRequest({
                        filters: {
                          ...generationRequest.filters,
                          minAmount: e.target.value
                        }
                      })
                    }
                  />
                </div>
              </div>
            </div>
          </div>
        )

      case 3:
        return (
          <div className="space-y-6">
            <div className="space-y-4">
              <h4 className="font-medium">Output Configuration</h4>
              <div className="space-y-2">
                <Label>Output Format</Label>
                <Select
                  value={
                    generationRequest.outputFormat || selectedTemplate?.format
                  }
                  onValueChange={(value) =>
                    updateGenerationRequest({ outputFormat: value })
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="PDF">PDF</SelectItem>
                    <SelectItem value="HTML">HTML</SelectItem>
                    <SelectItem value="EXCEL">Excel</SelectItem>
                    <SelectItem value="WORD">Word</SelectItem>
                    <SelectItem value="CSV">CSV</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="space-y-4">
              <h4 className="font-medium">Delivery Options</h4>
              <div className="space-y-3">
                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="download-immediately"
                    checked={
                      generationRequest.deliveryOptions.downloadImmediately
                    }
                    onCheckedChange={(checked) =>
                      updateGenerationRequest({
                        deliveryOptions: {
                          ...generationRequest.deliveryOptions,
                          downloadImmediately: checked as boolean
                        }
                      })
                    }
                  />
                  <Label
                    htmlFor="download-immediately"
                    className="flex items-center space-x-2"
                  >
                    <Download className="h-4 w-4" />
                    <span>Download immediately after generation</span>
                  </Label>
                </div>

                <div className="space-y-2">
                  <div className="flex items-center space-x-2">
                    <Mail className="h-4 w-4" />
                    <Label>Email Recipients (Optional)</Label>
                  </div>
                  <Input
                    placeholder="email1@example.com, email2@example.com"
                    value={generationRequest.deliveryOptions.emailRecipients.join(
                      ', '
                    )}
                    onChange={(e) =>
                      updateGenerationRequest({
                        deliveryOptions: {
                          ...generationRequest.deliveryOptions,
                          emailRecipients: e.target.value
                            .split(',')
                            .map((email) => email.trim())
                            .filter(Boolean)
                        }
                      })
                    }
                  />
                </div>

                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="schedule-generation"
                    checked={
                      generationRequest.deliveryOptions.scheduleGeneration
                    }
                    onCheckedChange={(checked) =>
                      updateGenerationRequest({
                        deliveryOptions: {
                          ...generationRequest.deliveryOptions,
                          scheduleGeneration: checked as boolean
                        }
                      })
                    }
                  />
                  <Label
                    htmlFor="schedule-generation"
                    className="flex items-center space-x-2"
                  >
                    <Clock className="h-4 w-4" />
                    <span>Schedule for later generation</span>
                  </Label>
                </div>

                {generationRequest.deliveryOptions.scheduleGeneration && (
                  <div className="ml-6 space-y-2">
                    <Select
                      value={generationRequest.deliveryOptions.scheduleType}
                      onValueChange={(
                        value: 'once' | 'daily' | 'weekly' | 'monthly'
                      ) =>
                        updateGenerationRequest({
                          deliveryOptions: {
                            ...generationRequest.deliveryOptions,
                            scheduleType: value
                          }
                        })
                      }
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Select schedule type" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="once">One-time</SelectItem>
                        <SelectItem value="daily">Daily</SelectItem>
                        <SelectItem value="weekly">Weekly</SelectItem>
                        <SelectItem value="monthly">Monthly</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                )}
              </div>
            </div>

            <div className="rounded-lg bg-blue-50 p-4 dark:bg-blue-900/20">
              <h4 className="mb-1 flex items-center space-x-2 font-medium text-blue-900 dark:text-blue-100">
                <AlertCircle className="h-4 w-4" />
                <span>Generation Summary</span>
              </h4>
              <div className="space-y-1 text-sm text-blue-800 dark:text-blue-200">
                <p>Template: {selectedTemplate?.name}</p>
                <p>
                  Format:{' '}
                  {generationRequest.outputFormat || selectedTemplate?.format}
                </p>
                <p>
                  Date Range:{' '}
                  {generationRequest.dateRange.start &&
                  generationRequest.dateRange.end
                    ? `${format(generationRequest.dateRange.start, 'MMM dd')} - ${format(generationRequest.dateRange.end, 'MMM dd, yyyy')}`
                    : 'All time'}
                </p>
                {generationRequest.deliveryOptions.emailRecipients.length >
                  0 && (
                  <p>
                    Email to:{' '}
                    {generationRequest.deliveryOptions.emailRecipients.length}{' '}
                    recipient(s)
                  </p>
                )}
              </div>
            </div>
          </div>
        )

      default:
        return null
    }
  }

  if (isGenerating) {
    return (
      <div className="mx-auto max-w-2xl py-16">
        <Card>
          <CardContent className="p-8 text-center">
            <div className="space-y-4">
              <Play className="mx-auto h-12 w-12 text-primary" />
              <h2 className="text-xl font-bold">Generating Report</h2>
              <p className="text-muted-foreground">
                Please wait while we generate your report...
              </p>
              <div className="space-y-2">
                <Progress value={generationProgress} className="h-3" />
                <p className="text-sm text-muted-foreground">
                  {generationProgress}% complete
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    )
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
          <h1 className="text-2xl font-bold">Generate Report</h1>
          <p className="text-muted-foreground">
            Configure and generate a new report from your template
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
            onClick={handleGenerate}
            disabled={!canProceed()}
            className="flex items-center space-x-2"
          >
            <Play className="h-4 w-4" />
            <span>Generate Report</span>
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
