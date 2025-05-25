'use client'

import { useState } from 'react'
import {
  WorkflowTemplate,
  TemplateCategory,
  TemplateComplexity
} from '@/core/domain/entities/workflow-template'
import { mockWorkflowTemplates } from '@/core/domain/mock-data/workflow-templates'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  Star,
  Download,
  Eye,
  Filter,
  Search,
  Play,
  Settings
} from 'lucide-react'
import { TemplateDetailDialog } from './template-detail-dialog'
import { TemplateInstantiationDialog } from './template-instantiation-dialog'

interface TemplateCatalogProps {
  onUseTemplate?: (
    template: WorkflowTemplate,
    parameters: Record<string, any>
  ) => void
}

export function TemplateCatalog({ onUseTemplate }: TemplateCatalogProps) {
  const [templates] = useState<WorkflowTemplate[]>(mockWorkflowTemplates)
  const [filteredTemplates, setFilteredTemplates] = useState<
    WorkflowTemplate[]
  >(mockWorkflowTemplates)
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedCategory, setSelectedCategory] = useState<string>('all')
  const [selectedComplexity, setSelectedComplexity] = useState<string>('all')
  const [selectedTemplate, setSelectedTemplate] =
    useState<WorkflowTemplate | null>(null)
  const [instantiationTemplate, setInstantiationTemplate] =
    useState<WorkflowTemplate | null>(null)
  const [showDetailDialog, setShowDetailDialog] = useState(false)
  const [showInstantiationDialog, setShowInstantiationDialog] = useState(false)

  const handleSearch = (query: string) => {
    setSearchQuery(query)
    applyFilters(query, selectedCategory, selectedComplexity)
  }

  const handleCategoryFilter = (category: string) => {
    setSelectedCategory(category)
    applyFilters(searchQuery, category, selectedComplexity)
  }

  const handleComplexityFilter = (complexity: string) => {
    setSelectedComplexity(complexity)
    applyFilters(searchQuery, selectedCategory, complexity)
  }

  const applyFilters = (
    query: string,
    category: string,
    complexity: string
  ) => {
    let filtered = templates

    if (query) {
      filtered = filtered.filter(
        (template) =>
          template.name.toLowerCase().includes(query.toLowerCase()) ||
          template.description.toLowerCase().includes(query.toLowerCase()) ||
          template.tags.some((tag) =>
            tag.toLowerCase().includes(query.toLowerCase())
          )
      )
    }

    if (category !== 'all') {
      filtered = filtered.filter((template) => template.category === category)
    }

    if (complexity !== 'all') {
      filtered = filtered.filter(
        (template) => template.metadata.complexity === complexity
      )
    }

    setFilteredTemplates(filtered)
  }

  const getComplexityColor = (complexity: TemplateComplexity) => {
    switch (complexity) {
      case 'SIMPLE':
        return 'bg-green-100 text-green-800'
      case 'MEDIUM':
        return 'bg-yellow-100 text-yellow-800'
      case 'COMPLEX':
        return 'bg-orange-100 text-orange-800'
      case 'ADVANCED':
        return 'bg-red-100 text-red-800'
      default:
        return 'bg-gray-100 text-gray-800'
    }
  }

  const getCategoryColor = (category: TemplateCategory) => {
    const colors = {
      payments: 'bg-blue-100 text-blue-800',
      onboarding: 'bg-purple-100 text-purple-800',
      compliance: 'bg-indigo-100 text-indigo-800',
      reconciliation: 'bg-cyan-100 text-cyan-800',
      reporting: 'bg-teal-100 text-teal-800',
      notifications: 'bg-pink-100 text-pink-800',
      integration: 'bg-amber-100 text-amber-800',
      custom: 'bg-gray-100 text-gray-800'
    }
    return colors[category] || 'bg-gray-100 text-gray-800'
  }

  const handleViewTemplate = (template: WorkflowTemplate) => {
    setSelectedTemplate(template)
    setShowDetailDialog(true)
  }

  const handleUseTemplate = (template: WorkflowTemplate) => {
    setInstantiationTemplate(template)
    setShowInstantiationDialog(true)
  }

  const handleInstantiateTemplate = (
    template: WorkflowTemplate,
    parameters: Record<string, any>
  ) => {
    onUseTemplate?.(template, parameters)
    setShowInstantiationDialog(false)
    setInstantiationTemplate(null)
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold">Workflow Templates</h1>
        <p className="text-muted-foreground">
          Browse and use pre-built workflow templates for common business
          processes
        </p>
      </div>

      {/* Filters */}
      <div className="flex flex-col gap-4 sm:flex-row">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
          <Input
            placeholder="Search templates..."
            value={searchQuery}
            onChange={(e) => handleSearch(e.target.value)}
            className="pl-10"
          />
        </div>
        <div className="flex gap-2">
          <Select value={selectedCategory} onValueChange={handleCategoryFilter}>
            <SelectTrigger className="w-40">
              <SelectValue placeholder="Category" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Categories</SelectItem>
              <SelectItem value="payments">Payments</SelectItem>
              <SelectItem value="onboarding">Onboarding</SelectItem>
              <SelectItem value="compliance">Compliance</SelectItem>
              <SelectItem value="reconciliation">Reconciliation</SelectItem>
              <SelectItem value="reporting">Reporting</SelectItem>
              <SelectItem value="notifications">Notifications</SelectItem>
              <SelectItem value="integration">Integration</SelectItem>
              <SelectItem value="custom">Custom</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={selectedComplexity}
            onValueChange={handleComplexityFilter}
          >
            <SelectTrigger className="w-40">
              <SelectValue placeholder="Complexity" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Complexity</SelectItem>
              <SelectItem value="SIMPLE">Simple</SelectItem>
              <SelectItem value="MEDIUM">Medium</SelectItem>
              <SelectItem value="COMPLEX">Complex</SelectItem>
              <SelectItem value="ADVANCED">Advanced</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* Results Count */}
      <div className="text-sm text-muted-foreground">
        Showing {filteredTemplates.length} of {templates.length} templates
      </div>

      {/* Template Grid */}
      <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
        {filteredTemplates.map((template) => (
          <Card key={template.id} className="transition-shadow hover:shadow-md">
            <CardHeader className="pb-3">
              <div className="flex items-start justify-between">
                <div className="flex-1">
                  <CardTitle className="line-clamp-1 text-lg">
                    {template.name}
                  </CardTitle>
                  <CardDescription className="mt-1 line-clamp-2">
                    {template.description}
                  </CardDescription>
                </div>
              </div>
              <div className="mt-3 flex items-center gap-2">
                <Badge className={getCategoryColor(template.category)}>
                  {template.category}
                </Badge>
                <Badge
                  className={getComplexityColor(template.metadata.complexity)}
                >
                  {template.metadata.complexity}
                </Badge>
              </div>
            </CardHeader>
            <CardContent className="pt-0">
              <div className="space-y-3">
                {/* Stats */}
                <div className="flex items-center justify-between text-sm text-muted-foreground">
                  <div className="flex items-center gap-1">
                    <Star className="h-3 w-3 fill-current text-yellow-500" />
                    <span>{template.rating}</span>
                  </div>
                  <div className="flex items-center gap-1">
                    <Download className="h-3 w-3" />
                    <span>{template.usageCount.toLocaleString()} uses</span>
                  </div>
                </div>

                {/* Tags */}
                <div className="flex flex-wrap gap-1">
                  {template.tags.slice(0, 3).map((tag) => (
                    <Badge key={tag} variant="secondary" className="text-xs">
                      {tag}
                    </Badge>
                  ))}
                  {template.tags.length > 3 && (
                    <Badge variant="secondary" className="text-xs">
                      +{template.tags.length - 3}
                    </Badge>
                  )}
                </div>

                {/* Duration */}
                <div className="text-xs text-muted-foreground">
                  Est. duration: {template.metadata.estimatedDuration}
                </div>

                {/* Actions */}
                <div className="flex gap-2 pt-2">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => handleViewTemplate(template)}
                    className="flex-1"
                  >
                    <Eye className="mr-1 h-3 w-3" />
                    View
                  </Button>
                  <Button
                    size="sm"
                    onClick={() => handleUseTemplate(template)}
                    className="flex-1"
                  >
                    <Play className="mr-1 h-3 w-3" />
                    Use
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Empty State */}
      {filteredTemplates.length === 0 && (
        <div className="py-12 text-center">
          <Filter className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
          <h3 className="mb-2 text-lg font-medium">No templates found</h3>
          <p className="text-muted-foreground">
            Try adjusting your search criteria or filters
          </p>
        </div>
      )}

      {/* Dialogs */}
      {selectedTemplate && (
        <TemplateDetailDialog
          open={showDetailDialog}
          onOpenChange={setShowDetailDialog}
          template={selectedTemplate}
          onUseTemplate={() => handleUseTemplate(selectedTemplate)}
        />
      )}

      {instantiationTemplate && (
        <TemplateInstantiationDialog
          open={showInstantiationDialog}
          onOpenChange={setShowInstantiationDialog}
          template={instantiationTemplate}
          onInstantiate={(parameters) =>
            handleInstantiateTemplate(instantiationTemplate, parameters)
          }
        />
      )}
    </div>
  )
}
