'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { ArrowLeft, Save, Eye } from 'lucide-react'

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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Stepper } from '@/components/ui/stepper'
import { PageHeader } from '@/components/page-header'

import { RouteTemplateLibrary } from '@/components/accounting/transaction-routes/route-template-library'
import {
  type RouteTemplate,
  type TransactionRoute
} from '@/components/accounting/mock/transaction-route-mock-data'

const steps = [
  { title: 'Basic Information', description: 'Name, description, and type' },
  { title: 'Template Selection', description: 'Choose a starting template' },
  {
    title: 'Route Configuration',
    description: 'Configure operations and rules'
  },
  { title: 'Review & Create', description: 'Review and finalize the route' }
]

export default function CreateTransactionRoutePage() {
  const router = useRouter()
  const [currentStep, setCurrentStep] = useState(0)
  const [selectedTemplate, setSelectedTemplate] =
    useState<RouteTemplate | null>(null)
  const [formData, setFormData] = useState({
    name: '',
    description: '',
    templateType: 'custom' as const,
    tags: [] as string[],
    metadata: {} as Record<string, any>
  })

  const handleBack = () => {
    if (currentStep > 0) {
      setCurrentStep(currentStep - 1)
    } else {
      router.push('/plugins/accounting/transaction-routes')
    }
  }

  const handleNext = () => {
    if (currentStep < steps.length - 1) {
      setCurrentStep(currentStep + 1)
    }
  }

  const handleTemplateSelect = (template: RouteTemplate) => {
    setSelectedTemplate(template)
    setFormData((prev) => ({
      ...prev,
      templateType: template.category as any,
      description: prev.description || template.description,
      tags: [...prev.tags, ...template.tags].filter(
        (tag, index, self) => self.indexOf(tag) === index
      ),
      metadata: { ...template.metadata, ...prev.metadata }
    }))
    handleNext()
  }

  const handleCreateRoute = () => {
    // In a real implementation, this would call an API to create the route
    const newRoute: Partial<TransactionRoute> = {
      name: formData.name,
      description: formData.description,
      templateType: formData.templateType,
      status: 'draft',
      tags: formData.tags,
      metadata: formData.metadata,
      operationRoutes: selectedTemplate?.operationRoutes || [],
      version: '1.0.0'
    }

    console.log('Creating transaction route:', newRoute)

    // Redirect to the designer page for the new route
    router.push('/plugins/accounting/transaction-routes/new-route-id/designer')
  }

  const canProceed = () => {
    switch (currentStep) {
      case 0:
        return formData.name.trim() !== '' && formData.description.trim() !== ''
      case 1:
        return selectedTemplate !== null
      case 2:
        return true // Configuration step is optional for now
      case 3:
        return true // Review step always allows proceeding
      default:
        return false
    }
  }

  const renderStepContent = () => {
    switch (currentStep) {
      case 0:
        return (
          <Card>
            <CardHeader>
              <CardTitle>Basic Information</CardTitle>
              <CardDescription>
                Provide basic details about your transaction route.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="name">Route Name *</Label>
                <Input
                  id="name"
                  placeholder="e.g., Customer Transfer Route"
                  value={formData.name}
                  onChange={(e) =>
                    setFormData((prev) => ({ ...prev, name: e.target.value }))
                  }
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="description">Description *</Label>
                <Textarea
                  id="description"
                  placeholder="Describe what this route is used for..."
                  value={formData.description}
                  onChange={(e) =>
                    setFormData((prev) => ({
                      ...prev,
                      description: e.target.value
                    }))
                  }
                  rows={3}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="templateType">Route Type</Label>
                <Select
                  value={formData.templateType}
                  onValueChange={(value: any) =>
                    setFormData((prev) => ({ ...prev, templateType: value }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select route type" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="transfer">Transfer</SelectItem>
                    <SelectItem value="payment">Payment</SelectItem>
                    <SelectItem value="adjustment">Adjustment</SelectItem>
                    <SelectItem value="fee">Fee</SelectItem>
                    <SelectItem value="refund">Refund</SelectItem>
                    <SelectItem value="custom">Custom</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label>Tags</Label>
                <div className="flex flex-wrap gap-2">
                  {formData.tags.map((tag, index) => (
                    <Badge
                      key={index}
                      variant="secondary"
                      className="cursor-pointer"
                      onClick={() =>
                        setFormData((prev) => ({
                          ...prev,
                          tags: prev.tags.filter((_, i) => i !== index)
                        }))
                      }
                    >
                      {tag} ×
                    </Badge>
                  ))}
                  <Input
                    placeholder="Add tag..."
                    className="h-8 w-32"
                    onKeyPress={(e) => {
                      if (e.key === 'Enter') {
                        const value = (
                          e.target as HTMLInputElement
                        ).value.trim()
                        if (value && !formData.tags.includes(value)) {
                          setFormData((prev) => ({
                            ...prev,
                            tags: [...prev.tags, value]
                          }))
                          ;(e.target as HTMLInputElement).value = ''
                        }
                      }
                    }}
                  />
                </div>
              </div>
            </CardContent>
          </Card>
        )

      case 1:
        return (
          <Card>
            <CardHeader>
              <CardTitle>Template Selection</CardTitle>
              <CardDescription>
                Choose a template to get started, or skip to create from
                scratch.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <RouteTemplateLibrary onSelectTemplate={handleTemplateSelect} />

              {selectedTemplate && (
                <div className="mt-6 rounded-lg border border-green-200 bg-green-50 p-4">
                  <h4 className="font-medium text-green-800">
                    Selected Template:
                  </h4>
                  <p className="mt-1 text-sm text-green-700">
                    {selectedTemplate.name} - {selectedTemplate.description}
                  </p>
                  <div className="mt-2 flex items-center space-x-2">
                    <Badge className="bg-green-100 text-green-800">
                      {selectedTemplate.category}
                    </Badge>
                    <span className="text-xs text-green-600">
                      {selectedTemplate.operationRoutes.length} operations
                    </span>
                  </div>
                </div>
              )}

              <div className="mt-4 text-center">
                <Button variant="outline" onClick={handleNext}>
                  Skip Template (Create from Scratch)
                </Button>
              </div>
            </CardContent>
          </Card>
        )

      case 2:
        return (
          <Card>
            <CardHeader>
              <CardTitle>Route Configuration</CardTitle>
              <CardDescription>
                Configure additional settings and metadata for your route.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>Daily Transaction Limit</Label>
                  <Input
                    type="number"
                    placeholder="e.g., 10000"
                    onChange={(e) =>
                      setFormData((prev) => ({
                        ...prev,
                        metadata: {
                          ...prev.metadata,
                          dailyLimit: parseFloat(e.target.value) || undefined
                        }
                      }))
                    }
                  />
                </div>

                <div className="space-y-2">
                  <Label>Monthly Transaction Limit</Label>
                  <Input
                    type="number"
                    placeholder="e.g., 100000"
                    onChange={(e) =>
                      setFormData((prev) => ({
                        ...prev,
                        metadata: {
                          ...prev.metadata,
                          monthlyLimit: parseFloat(e.target.value) || undefined
                        }
                      }))
                    }
                  />
                </div>
              </div>

              <div className="space-y-2">
                <Label>Supported Currencies</Label>
                <Input
                  placeholder="e.g., USD,BRL,EUR (comma-separated)"
                  onChange={(e) =>
                    setFormData((prev) => ({
                      ...prev,
                      metadata: {
                        ...prev.metadata,
                        supportedCurrencies: e.target.value
                          .split(',')
                          .map((c) => c.trim())
                          .filter(Boolean)
                      }
                    }))
                  }
                />
              </div>

              <div className="space-y-2">
                <Label>Additional Metadata (JSON)</Label>
                <Textarea
                  placeholder='{"requiresKyc": true, "maxRetries": 3}'
                  rows={4}
                  onChange={(e) => {
                    try {
                      const additionalMetadata = JSON.parse(
                        e.target.value || '{}'
                      )
                      setFormData((prev) => ({
                        ...prev,
                        metadata: { ...prev.metadata, ...additionalMetadata }
                      }))
                    } catch {
                      // Invalid JSON, ignore
                    }
                  }}
                />
              </div>
            </CardContent>
          </Card>
        )

      case 3:
        return (
          <Card>
            <CardHeader>
              <CardTitle>Review & Create</CardTitle>
              <CardDescription>
                Review your transaction route configuration before creating.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-4">
                <div>
                  <h4 className="font-medium">Basic Information</h4>
                  <div className="mt-2 text-sm text-muted-foreground">
                    <p>
                      <strong>Name:</strong> {formData.name}
                    </p>
                    <p>
                      <strong>Description:</strong> {formData.description}
                    </p>
                    <p>
                      <strong>Type:</strong> {formData.templateType}
                    </p>
                  </div>
                </div>

                {formData.tags.length > 0 && (
                  <div>
                    <h4 className="font-medium">Tags</h4>
                    <div className="mt-2 flex flex-wrap gap-1">
                      {formData.tags.map((tag, index) => (
                        <Badge key={index} variant="outline">
                          {tag}
                        </Badge>
                      ))}
                    </div>
                  </div>
                )}

                {selectedTemplate && (
                  <div>
                    <h4 className="font-medium">Template</h4>
                    <div className="mt-2 text-sm text-muted-foreground">
                      <p>
                        <strong>Selected:</strong> {selectedTemplate.name}
                      </p>
                      <p>
                        <strong>Operations:</strong>{' '}
                        {selectedTemplate.operationRoutes.length}
                      </p>
                    </div>
                  </div>
                )}

                {Object.keys(formData.metadata).length > 0 && (
                  <div>
                    <h4 className="font-medium">Configuration</h4>
                    <div className="mt-2 rounded bg-muted p-3 text-xs">
                      <pre>{JSON.stringify(formData.metadata, null, 2)}</pre>
                    </div>
                  </div>
                )}
              </div>

              <div className="flex justify-center space-x-4">
                <Button
                  variant="outline"
                  onClick={() => console.log('Preview')}
                >
                  <Eye className="mr-2 h-4 w-4" />
                  Preview Route
                </Button>
                <Button onClick={handleCreateRoute}>
                  <Save className="mr-2 h-4 w-4" />
                  Create Route
                </Button>
              </div>
            </CardContent>
          </Card>
        )

      default:
        return null
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <div className="flex items-center space-x-4">
          <Button variant="ghost" size="sm" onClick={handleBack}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <PageHeader.InfoTitle>
              Create Transaction Route
            </PageHeader.InfoTitle>
            <PageHeader.InfoTooltip>
              Create a new transaction route with automated operation mapping.
            </PageHeader.InfoTooltip>
          </div>
        </div>
      </PageHeader.Root>

      <Card>
        <CardHeader>
          <Stepper
            steps={steps}
            currentStep={currentStep}
            onStepClick={setCurrentStep}
          />
        </CardHeader>
      </Card>

      {renderStepContent()}

      <div className="flex justify-between">
        <Button variant="outline" onClick={handleBack}>
          {currentStep === 0 ? 'Cancel' : 'Back'}
        </Button>

        {currentStep < steps.length - 1 && (
          <Button onClick={handleNext} disabled={!canProceed()}>
            Next
          </Button>
        )}
      </div>
    </div>
  )
}
