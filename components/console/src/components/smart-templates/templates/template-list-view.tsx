'use client'

import { useState } from 'react'
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
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  Search,
  Plus,
  MoreHorizontal,
  Edit,
  Copy,
  Trash2,
  Play,
  FileText,
  Grid3X3,
  List,
  Filter,
  Download,
  Eye
} from 'lucide-react'
import {
  Template,
  TemplateCategory,
  TemplateStatus
} from '@/core/domain/entities/template'
import { mockTemplates } from '@/lib/mock-data/smart-templates'

type ViewMode = 'grid' | 'list'

const getInitials = (name: string) => {
  return name
    .split(' ')
    .map((word) => word.charAt(0))
    .join('')
    .toUpperCase()
    .slice(0, 2)
}

export function TemplateListView() {
  const router = useRouter()
  const [templates] = useState<Template[]>(mockTemplates)
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedCategory, setSelectedCategory] = useState<
    TemplateCategory | 'ALL'
  >('ALL')
  const [selectedStatus, setSelectedStatus] = useState<TemplateStatus | 'ALL'>(
    'ALL'
  )
  const [viewMode, setViewMode] = useState<ViewMode>('grid')

  const statusColors = {
    draft: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
    active: 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200',
    inactive:
      'bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-200',
    archived: 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200'
  } as const

  const categoryColors: Record<string, string> = {
    FINANCIAL: 'bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-200',
    OPERATIONAL:
      'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200',
    COMPLIANCE:
      'bg-purple-100 text-purple-800 dark:bg-purple-800 dark:text-purple-200',
    MARKETING:
      'bg-orange-100 text-orange-800 dark:bg-orange-800 dark:text-orange-200',
    CUSTOM: 'bg-pink-100 text-pink-800 dark:bg-pink-800 dark:text-pink-200'
  }

  const filteredTemplates = templates.filter((template) => {
    const matchesSearch =
      template.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      template.description?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      template.tags.some((tag) =>
        tag.toLowerCase().includes(searchQuery.toLowerCase())
      )
    const matchesCategory =
      selectedCategory === 'ALL' || template.category === selectedCategory
    const matchesStatus =
      selectedStatus === 'ALL' || template.status === selectedStatus

    return matchesSearch && matchesCategory && matchesStatus
  })

  const formatDate = (date: string) => {
    return new Date(date).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric'
    })
  }

  const handleCreateTemplate = () => {
    router.push('/plugins/smart-templates/templates/create')
  }

  const handleTemplateClick = (templateId: string) => {
    router.push(`/plugins/smart-templates/templates/${templateId}`)
  }

  const handleEdit = (templateId: string, e: React.MouseEvent) => {
    e.stopPropagation()
    router.push(`/plugins/smart-templates/templates/${templateId}/edit`)
  }

  const handleDuplicate = (templateId: string, e: React.MouseEvent) => {
    e.stopPropagation()
    console.log('Duplicating template:', templateId)
  }

  const handleDelete = (templateId: string, e: React.MouseEvent) => {
    e.stopPropagation()
    if (confirm('Are you sure you want to delete this template?')) {
      console.log('Deleting template:', templateId)
    }
  }

  const handleGenerateReport = (templateId: string, e: React.MouseEvent) => {
    e.stopPropagation()
    router.push(
      `/plugins/smart-templates/reports/generate?templateId=${templateId}`
    )
  }

  const renderGridView = () => (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {filteredTemplates.map((template) => (
        <Card
          key={template.id}
          className="cursor-pointer transition-shadow hover:shadow-md"
          onClick={() => handleTemplateClick(template.id)}
        >
          <CardHeader className="pb-3">
            <div className="flex items-start justify-between">
              <div className="min-w-0 flex-1">
                <CardTitle className="truncate text-base">
                  {template.name}
                </CardTitle>
                <div className="mt-1 flex items-center space-x-2">
                  <Badge
                    className={categoryColors[template.category]}
                    variant="secondary"
                  >
                    {template.category}
                  </Badge>
                  <Badge className={statusColors[template.status]}>
                    {template.status}
                  </Badge>
                </div>
              </div>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                    <MoreHorizontal className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={(e) => handleEdit(template.id, e)}>
                    <Edit className="mr-2 h-4 w-4" />
                    Edit
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={(e) => handleGenerateReport(template.id, e)}
                  >
                    <Play className="mr-2 h-4 w-4" />
                    Generate Report
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={(e) => handleDuplicate(template.id, e)}
                  >
                    <Copy className="mr-2 h-4 w-4" />
                    Duplicate
                  </DropdownMenuItem>
                  <DropdownMenuItem>
                    <Download className="mr-2 h-4 w-4" />
                    Export
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    onClick={(e) => handleDelete(template.id, e)}
                    className="text-red-600"
                  >
                    <Trash2 className="mr-2 h-4 w-4" />
                    Delete
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </CardHeader>
          <CardContent className="pt-0">
            <CardDescription className="mb-3 line-clamp-2">
              {template.description}
            </CardDescription>

            <div className="mb-3 flex items-center justify-between text-sm text-muted-foreground">
              <span>Format: {template.format || 'Unknown'}</span>
              <span>v{template.version || '1.0'}</span>
            </div>

            <div className="mb-3 flex flex-wrap gap-1">
              {template.tags.slice(0, 2).map((tag) => (
                <Badge key={tag} variant="outline" className="text-xs">
                  {tag}
                </Badge>
              ))}
              {template.tags.length > 2 && (
                <Badge variant="outline" className="text-xs">
                  +{template.tags.length - 2}
                </Badge>
              )}
            </div>

            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-2">
                <Avatar className="h-6 w-6">
                  <AvatarFallback>
                    {getInitials(template.createdBy)}
                  </AvatarFallback>
                </Avatar>
                <span className="text-xs text-muted-foreground">
                  {template.createdBy}
                </span>
              </div>
              <span className="text-xs text-muted-foreground">
                {formatDate(template.updatedAt)}
              </span>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )

  const renderListView = () => (
    <div className="space-y-2">
      {filteredTemplates.map((template) => (
        <Card
          key={template.id}
          className="cursor-pointer transition-shadow hover:shadow-sm"
          onClick={() => handleTemplateClick(template.id)}
        >
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div className="flex min-w-0 flex-1 items-center space-x-4">
                <FileText className="h-5 w-5 flex-shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <div className="mb-1 flex items-center space-x-3">
                    <h3 className="truncate font-medium">{template.name}</h3>
                    <Badge
                      className={categoryColors[template.category]}
                      variant="secondary"
                    >
                      {template.category}
                    </Badge>
                    <Badge className={statusColors[template.status]}>
                      {template.status}
                    </Badge>
                  </div>
                  <p className="truncate text-sm text-muted-foreground">
                    {template.description}
                  </p>
                </div>
              </div>

              <div className="flex flex-shrink-0 items-center space-x-4">
                <div className="text-right">
                  <p className="text-sm font-medium">
                    {template.format || 'Unknown'}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    v{template.version || '1.0'}
                  </p>
                </div>
                <div className="text-right">
                  <p className="text-sm font-medium">{template.createdBy}</p>
                  <p className="text-xs text-muted-foreground">
                    {formatDate(template.updatedAt)}
                  </p>
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                      <MoreHorizontal className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      onClick={(e) => handleEdit(template.id, e)}
                    >
                      <Edit className="mr-2 h-4 w-4" />
                      Edit
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={(e) => handleGenerateReport(template.id, e)}
                    >
                      <Play className="mr-2 h-4 w-4" />
                      Generate Report
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={(e) => handleDuplicate(template.id, e)}
                    >
                      <Copy className="mr-2 h-4 w-4" />
                      Duplicate
                    </DropdownMenuItem>
                    <DropdownMenuItem>
                      <Eye className="mr-2 h-4 w-4" />
                      Preview
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      onClick={(e) => handleDelete(template.id, e)}
                      className="text-red-600"
                    >
                      <Trash2 className="mr-2 h-4 w-4" />
                      Delete
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Templates</h1>
          <p className="text-muted-foreground">
            Manage your document templates and generate reports
          </p>
        </div>
        <Button
          onClick={handleCreateTemplate}
          className="flex items-center space-x-2"
        >
          <Plus className="h-4 w-4" />
          <span>Create Template</span>
        </Button>
      </div>

      {/* Filters and Search */}
      <div className="flex flex-col gap-4 sm:flex-row">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
          <Input
            placeholder="Search templates..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-10"
          />
        </div>

        <div className="flex items-center space-x-2">
          <Select
            value={selectedCategory}
            onValueChange={(value: TemplateCategory | 'ALL') =>
              setSelectedCategory(value)
            }
          >
            <SelectTrigger className="w-[140px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="ALL">All Categories</SelectItem>
              <SelectItem value="FINANCIAL">Financial</SelectItem>
              <SelectItem value="OPERATIONAL">Operational</SelectItem>
              <SelectItem value="COMPLIANCE">Compliance</SelectItem>
              <SelectItem value="MARKETING">Marketing</SelectItem>
              <SelectItem value="CUSTOM">Custom</SelectItem>
            </SelectContent>
          </Select>

          <Select
            value={selectedStatus}
            onValueChange={(value: TemplateStatus | 'ALL') =>
              setSelectedStatus(value)
            }
          >
            <SelectTrigger className="w-[120px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="ALL">All Status</SelectItem>
              <SelectItem value="DRAFT">Draft</SelectItem>
              <SelectItem value="ACTIVE">Active</SelectItem>
              <SelectItem value="INACTIVE">Inactive</SelectItem>
              <SelectItem value="ARCHIVED">Archived</SelectItem>
            </SelectContent>
          </Select>

          <div className="flex items-center rounded-md border">
            <Button
              variant={viewMode === 'grid' ? 'default' : 'ghost'}
              size="sm"
              onClick={() => setViewMode('grid')}
              className="rounded-r-none"
            >
              <Grid3X3 className="h-4 w-4" />
            </Button>
            <Button
              variant={viewMode === 'list' ? 'default' : 'ghost'}
              size="sm"
              onClick={() => setViewMode('list')}
              className="rounded-l-none"
            >
              <List className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>

      {/* Results Summary */}
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          {filteredTemplates.length} template
          {filteredTemplates.length !== 1 ? 's' : ''} found
        </p>
        {(searchQuery ||
          selectedCategory !== 'ALL' ||
          selectedStatus !== 'ALL') && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setSearchQuery('')
              setSelectedCategory('ALL')
              setSelectedStatus('ALL')
            }}
            className="flex items-center space-x-2"
          >
            <Filter className="h-4 w-4" />
            <span>Clear Filters</span>
          </Button>
        )}
      </div>

      {/* Templates */}
      {filteredTemplates.length === 0 ? (
        <Card>
          <CardContent className="py-16">
            <div className="text-center">
              <FileText className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
              <h3 className="mb-2 text-lg font-medium">No templates found</h3>
              <p className="mb-4 text-muted-foreground">
                {searchQuery ||
                selectedCategory !== 'ALL' ||
                selectedStatus !== 'ALL'
                  ? 'Try adjusting your search criteria or filters'
                  : 'Get started by creating your first template'}
              </p>
              {!searchQuery &&
                selectedCategory === 'ALL' &&
                selectedStatus === 'ALL' && (
                  <Button onClick={handleCreateTemplate}>
                    <Plus className="mr-2 h-4 w-4" />
                    Create Template
                  </Button>
                )}
            </div>
          </CardContent>
        </Card>
      ) : viewMode === 'grid' ? (
        renderGridView()
      ) : (
        renderListView()
      )}
    </div>
  )
}
