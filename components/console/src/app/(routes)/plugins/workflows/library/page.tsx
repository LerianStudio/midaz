'use client'

import { useState, useEffect, useMemo } from 'react'
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
import { Checkbox } from '@/components/ui/checkbox'
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
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Calendar } from '@/components/ui/calendar'
import {
  Popover,
  PopoverContent,
  PopoverTrigger
} from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'
import { format } from 'date-fns'
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
  Eye,
  Upload,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
  CalendarIcon,
  X,
  CheckCircle,
  Clock,
  Activity,
  Power,
  FileDown,
  Zap
} from 'lucide-react'
import { Workflow, WorkflowStatus } from '@/core/domain/entities/workflow'
import {
  getWorkflows,
  deleteWorkflow,
  updateWorkflowStatus
} from '@/app/actions/workflows'
import { useToast } from '@/hooks/use-toast'
import { WorkflowImportExport } from '@/components/workflows/library/workflow-import-export'
import {
  WorkflowAdvancedSearch,
  SearchFilters
} from '@/components/workflows/library/workflow-advanced-search'

type ViewMode = 'grid' | 'list'
type SortField =
  | 'name'
  | 'createdAt'
  | 'updatedAt'
  | 'executionCount'
  | 'successRate'
type SortOrder = 'asc' | 'desc'

interface DateRange {
  from: Date | undefined
  to: Date | undefined
}

const workflowCategories = [
  { value: 'ALL', label: 'All Categories' },
  { value: 'payments', label: 'Payment' },
  { value: 'onboarding', label: 'Onboarding' },
  { value: 'reconciliation', label: 'Reconciliation' },
  { value: 'compliance', label: 'Compliance' },
  { value: 'reporting', label: 'Reporting' },
  { value: 'notifications', label: 'Notifications' },
  { value: 'data-processing', label: 'Data Processing' },
  { value: 'integration', label: 'Integration' },
  { value: 'other', label: 'Other' }
]

const executionFrequencies = [
  { value: 'ALL', label: 'All Frequencies' },
  { value: 'high', label: 'High (>1000)', min: 1000 },
  { value: 'medium', label: 'Medium (100-1000)', min: 100, max: 1000 },
  { value: 'low', label: 'Low (<100)', max: 100 },
  { value: 'never', label: 'Never Executed', exact: 0 }
]

const getInitials = (name: string) => {
  return name
    .split(' ')
    .map((word) => word.charAt(0))
    .join('')
    .toUpperCase()
    .slice(0, 2)
}

const statusColors = {
  ACTIVE: 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200',
  INACTIVE:
    'bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-200',
  DRAFT: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
  DEPRECATED: 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200'
}

const statusIcons = {
  ACTIVE: <CheckCircle className="h-3 w-3" />,
  INACTIVE: <Clock className="h-3 w-3" />,
  DRAFT: <FileText className="h-3 w-3" />,
  DEPRECATED: <X className="h-3 w-3" />
}

export default function WorkflowLibraryPage() {
  const router = useRouter()
  const { toast } = useToast()
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [loading, setLoading] = useState(true)
  const [searchQuery, setSearchQuery] = useState('')
  const [advancedFilters, setAdvancedFilters] = useState<SearchFilters>({})
  const [selectedStatus, setSelectedStatus] = useState<WorkflowStatus | 'ALL'>(
    'ALL'
  )
  const [selectedCategory, setSelectedCategory] = useState<string>('ALL')
  const [selectedFrequency, setSelectedFrequency] = useState<string>('ALL')
  const [selectedAuthor, setSelectedAuthor] = useState<string>('ALL')
  const [dateRange, setDateRange] = useState<DateRange>({
    from: undefined,
    to: undefined
  })
  const [viewMode, setViewMode] = useState<ViewMode>('grid')
  const [sortField, setSortField] = useState<SortField>('updatedAt')
  const [sortOrder, setSortOrder] = useState<SortOrder>('desc')
  const [selectedWorkflows, setSelectedWorkflows] = useState<Set<string>>(
    new Set()
  )
  const [currentPage, setCurrentPage] = useState(1)
  const [itemsPerPage, setItemsPerPage] = useState(12)
  const [showImportDialog, setShowImportDialog] = useState(false)
  const [showExportDialog, setShowExportDialog] = useState(false)
  const [selectedWorkflowForExport, setSelectedWorkflowForExport] = useState<
    Workflow | undefined
  >()

  // Fetch workflows
  useEffect(() => {
    loadWorkflows()
  }, [])

  const loadWorkflows = async () => {
    setLoading(true)
    try {
      const result = await getWorkflows({
        organizationId: 'default',
        limit: 1000,
        page: 1
      })
      if (result.success && result.data) {
        setWorkflows(result.data.workflows)
      } else {
        toast({
          title: 'Error',
          description: result.error || 'Failed to load workflows',
          variant: 'destructive'
        })
      }
    } catch (error) {
      toast({
        title: 'Error',
        description: 'Failed to load workflows',
        variant: 'destructive'
      })
    } finally {
      setLoading(false)
    }
  }

  // Get unique authors for filter
  const authors = useMemo(() => {
    const uniqueAuthors = new Set(
      workflows.map((w) => w.metadata.author || w.createdBy)
    )
    return ['ALL', ...Array.from(uniqueAuthors)].filter(Boolean)
  }, [workflows])

  // Filter and sort workflows
  const filteredAndSortedWorkflows = useMemo(() => {
    let filtered = workflows.filter((workflow) => {
      // Search filter
      const matchesSearch =
        searchQuery === '' ||
        workflow.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        workflow.description
          ?.toLowerCase()
          .includes(searchQuery.toLowerCase()) ||
        workflow.metadata.tags.some((tag) =>
          tag.toLowerCase().includes(searchQuery.toLowerCase())
        )

      // Advanced filters from search component
      const matchesAdvancedFilters = (() => {
        // Status from advanced filters
        if (advancedFilters.status && advancedFilters.status.length > 0) {
          if (!advancedFilters.status.includes(workflow.status)) return false
        }

        // Categories from advanced filters
        if (
          advancedFilters.categories &&
          advancedFilters.categories.length > 0
        ) {
          if (
            !workflow.metadata.category ||
            !advancedFilters.categories.includes(workflow.metadata.category)
          ) {
            return false
          }
        }

        // Tags from advanced filters
        if (advancedFilters.tags && advancedFilters.tags.length > 0) {
          const hasMatchingTag = advancedFilters.tags.some((tag) =>
            workflow.metadata.tags.includes(tag)
          )
          if (!hasMatchingTag) return false
        }

        // Authors from advanced filters
        if (advancedFilters.authors && advancedFilters.authors.length > 0) {
          const workflowAuthor = workflow.metadata.author || workflow.createdBy
          if (!advancedFilters.authors.includes(workflowAuthor)) return false
        }

        // Execution range from advanced filters
        if (advancedFilters.executionRange) {
          const { min, max } = advancedFilters.executionRange
          if (min !== undefined && workflow.executionCount < min) return false
          if (max !== undefined && workflow.executionCount > max) return false
        }

        // Date range from advanced filters
        if (advancedFilters.dateRange) {
          const workflowDate = new Date(workflow.createdAt)
          if (
            advancedFilters.dateRange.from &&
            workflowDate < advancedFilters.dateRange.from
          )
            return false
          if (
            advancedFilters.dateRange.to &&
            workflowDate > advancedFilters.dateRange.to
          )
            return false
        }

        return true
      })()

      // Status filter (from dropdown)
      const matchesStatus =
        selectedStatus === 'ALL' || workflow.status === selectedStatus

      // Category filter (from dropdown)
      const matchesCategory =
        selectedCategory === 'ALL' ||
        workflow.metadata.category === selectedCategory

      // Author filter (from dropdown)
      const workflowAuthor = workflow.metadata.author || workflow.createdBy
      const matchesAuthor =
        selectedAuthor === 'ALL' || workflowAuthor === selectedAuthor

      // Execution frequency filter
      const matchesFrequency = (() => {
        if (selectedFrequency === 'ALL') return true
        const freq = executionFrequencies.find(
          (f) => f.value === selectedFrequency
        )
        if (!freq) return true

        if (freq.exact !== undefined) {
          return workflow.executionCount === freq.exact
        }
        if (freq.min !== undefined && freq.max !== undefined) {
          return (
            workflow.executionCount >= freq.min &&
            workflow.executionCount < freq.max
          )
        }
        if (freq.min !== undefined) {
          return workflow.executionCount >= freq.min
        }
        if (freq.max !== undefined) {
          return workflow.executionCount < freq.max
        }
        return true
      })()

      // Date range filter
      const matchesDateRange = (() => {
        if (!dateRange.from && !dateRange.to) return true
        const workflowDate = new Date(workflow.createdAt)
        if (dateRange.from && workflowDate < dateRange.from) return false
        if (dateRange.to && workflowDate > dateRange.to) return false
        return true
      })()

      return (
        matchesSearch &&
        matchesAdvancedFilters &&
        matchesStatus &&
        matchesCategory &&
        matchesAuthor &&
        matchesFrequency &&
        matchesDateRange
      )
    })

    // Sort workflows
    filtered.sort((a, b) => {
      let aValue: any
      let bValue: any

      switch (sortField) {
        case 'name':
          aValue = a.name.toLowerCase()
          bValue = b.name.toLowerCase()
          break
        case 'createdAt':
          aValue = new Date(a.createdAt).getTime()
          bValue = new Date(b.createdAt).getTime()
          break
        case 'updatedAt':
          aValue = new Date(a.updatedAt).getTime()
          bValue = new Date(b.updatedAt).getTime()
          break
        case 'executionCount':
          aValue = a.executionCount
          bValue = b.executionCount
          break
        case 'successRate':
          aValue = a.successRate
          bValue = b.successRate
          break
        default:
          return 0
      }

      if (sortOrder === 'asc') {
        return aValue > bValue ? 1 : -1
      } else {
        return aValue < bValue ? 1 : -1
      }
    })

    return filtered
  }, [
    workflows,
    searchQuery,
    advancedFilters,
    selectedStatus,
    selectedCategory,
    selectedAuthor,
    selectedFrequency,
    dateRange,
    sortField,
    sortOrder
  ])

  // Pagination
  const paginatedWorkflows = useMemo(() => {
    const startIndex = (currentPage - 1) * itemsPerPage
    const endIndex = startIndex + itemsPerPage
    return filteredAndSortedWorkflows.slice(startIndex, endIndex)
  }, [filteredAndSortedWorkflows, currentPage, itemsPerPage])

  const totalPages = Math.ceil(filteredAndSortedWorkflows.length / itemsPerPage)

  // Handlers
  const handleAdvancedSearch = (query: string, filters: SearchFilters) => {
    setSearchQuery(query)
    setAdvancedFilters(filters)
    // Reset page when searching
    setCurrentPage(1)
  }
  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedWorkflows(new Set(paginatedWorkflows.map((w) => w.id)))
    } else {
      setSelectedWorkflows(new Set())
    }
  }

  const handleSelectWorkflow = (workflowId: string, checked: boolean) => {
    const newSelection = new Set(selectedWorkflows)
    if (checked) {
      newSelection.add(workflowId)
    } else {
      newSelection.delete(workflowId)
    }
    setSelectedWorkflows(newSelection)
  }

  const handleBulkExport = () => {
    if (selectedWorkflows.size === 0) {
      toast({
        title: 'No workflows selected',
        description: 'Please select at least one workflow to export',
        variant: 'destructive'
      })
      return
    }

    // In a real implementation, this would export multiple workflows
    const workflowsToExport = workflows.filter((w) =>
      selectedWorkflows.has(w.id)
    )
    console.log('Exporting workflows:', workflowsToExport)
    toast({
      title: 'Exporting workflows',
      description: `Exporting ${selectedWorkflows.size} workflow(s)...`
    })
  }

  const handleBulkDelete = async () => {
    if (selectedWorkflows.size === 0) {
      toast({
        title: 'No workflows selected',
        description: 'Please select at least one workflow to delete',
        variant: 'destructive'
      })
      return
    }

    if (
      !confirm(
        `Are you sure you want to delete ${selectedWorkflows.size} workflow(s)?`
      )
    ) {
      return
    }

    // Delete selected workflows
    for (const workflowId of selectedWorkflows) {
      const result = await deleteWorkflow(workflowId)
      if (!result.success) {
        toast({
          title: 'Error',
          description: `Failed to delete workflow: ${result.error}`,
          variant: 'destructive'
        })
      }
    }

    toast({
      title: 'Success',
      description: `Deleted ${selectedWorkflows.size} workflow(s)`
    })
    setSelectedWorkflows(new Set())
    loadWorkflows()
  }

  const handleBulkStatusChange = async (status: WorkflowStatus) => {
    if (selectedWorkflows.size === 0) {
      toast({
        title: 'No workflows selected',
        description: 'Please select at least one workflow to update',
        variant: 'destructive'
      })
      return
    }

    // Update status for selected workflows
    for (const workflowId of selectedWorkflows) {
      const result = await updateWorkflowStatus(workflowId, status)
      if (!result.success) {
        toast({
          title: 'Error',
          description: `Failed to update workflow status: ${result.error}`,
          variant: 'destructive'
        })
      }
    }

    toast({
      title: 'Success',
      description: `Updated ${selectedWorkflows.size} workflow(s) to ${status}`
    })
    setSelectedWorkflows(new Set())
    loadWorkflows()
  }

  const handleSort = (field: SortField) => {
    if (field === sortField) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc')
    } else {
      setSortField(field)
      setSortOrder('asc')
    }
  }

  const clearFilters = () => {
    setSearchQuery('')
    setSelectedStatus('ALL')
    setSelectedCategory('ALL')
    setSelectedFrequency('ALL')
    setSelectedAuthor('ALL')
    setDateRange({ from: undefined, to: undefined })
  }

  const hasActiveFilters =
    searchQuery ||
    selectedStatus !== 'ALL' ||
    selectedCategory !== 'ALL' ||
    selectedFrequency !== 'ALL' ||
    selectedAuthor !== 'ALL' ||
    dateRange.from ||
    dateRange.to

  const formatDate = (date: string) => {
    return new Date(date).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric'
    })
  }

  const renderSortIcon = (field: SortField) => {
    if (sortField !== field) {
      return <ArrowUpDown className="ml-2 h-4 w-4 text-muted-foreground" />
    }
    return sortOrder === 'asc' ? (
      <ArrowUp className="ml-2 h-4 w-4" />
    ) : (
      <ArrowDown className="ml-2 h-4 w-4" />
    )
  }

  const renderGridView = () => (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {paginatedWorkflows.map((workflow) => (
        <Card
          key={workflow.id}
          className={cn(
            'cursor-pointer transition-all hover:shadow-md',
            selectedWorkflows.has(workflow.id) && 'ring-2 ring-primary'
          )}
          onClick={() =>
            router.push(`/plugins/workflows/library/${workflow.id}`)
          }
        >
          <CardHeader className="pb-3">
            <div className="flex items-start justify-between">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <Checkbox
                    checked={selectedWorkflows.has(workflow.id)}
                    onCheckedChange={(checked) =>
                      handleSelectWorkflow(workflow.id, checked as boolean)
                    }
                    onClick={(e) => e.stopPropagation()}
                  />
                  <CardTitle className="truncate text-base">
                    {workflow.name}
                  </CardTitle>
                </div>
                <div className="mt-2 flex items-center gap-2">
                  <Badge
                    className={statusColors[workflow.status]}
                    variant="secondary"
                  >
                    <span className="flex items-center gap-1">
                      {statusIcons[workflow.status]}
                      {workflow.status}
                    </span>
                  </Badge>
                  {workflow.metadata.category && (
                    <Badge variant="outline" className="text-xs">
                      {workflow.metadata.category}
                    </Badge>
                  )}
                </div>
              </div>
              <DropdownMenu>
                <DropdownMenuTrigger
                  asChild
                  onClick={(e) => e.stopPropagation()}
                >
                  <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                    <MoreHorizontal className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    onClick={(e) => {
                      e.stopPropagation()
                      router.push(`/plugins/workflows/library/${workflow.id}`)
                    }}
                  >
                    <Eye className="mr-2 h-4 w-4" />
                    View Details
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={(e) => {
                      e.stopPropagation()
                      router.push(
                        `/plugins/workflows/library/${workflow.id}/designer`
                      )
                    }}
                  >
                    <Edit className="mr-2 h-4 w-4" />
                    Edit
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={(e) => {
                      e.stopPropagation()
                      router.push(
                        `/plugins/workflows/executions/start?workflowId=${workflow.id}`
                      )
                    }}
                  >
                    <Play className="mr-2 h-4 w-4" />
                    Execute
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={(e) => {
                      e.stopPropagation()
                      setSelectedWorkflowForExport(workflow)
                      setShowExportDialog(true)
                    }}
                  >
                    <Download className="mr-2 h-4 w-4" />
                    Export
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    onClick={(e) => {
                      e.stopPropagation()
                      console.log('Duplicating workflow:', workflow.id)
                    }}
                  >
                    <Copy className="mr-2 h-4 w-4" />
                    Duplicate
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={async (e) => {
                      e.stopPropagation()
                      if (
                        confirm(
                          'Are you sure you want to delete this workflow?'
                        )
                      ) {
                        const result = await deleteWorkflow(workflow.id)
                        if (result.success) {
                          toast({
                            title: 'Success',
                            description: 'Workflow deleted successfully'
                          })
                          loadWorkflows()
                        } else {
                          toast({
                            title: 'Error',
                            description: result.error,
                            variant: 'destructive'
                          })
                        }
                      }
                    }}
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
              {workflow.description || 'No description available'}
            </CardDescription>

            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-2 text-sm">
                <div className="flex items-center gap-1">
                  <Activity className="h-3 w-3 text-muted-foreground" />
                  <span className="font-medium">{workflow.executionCount}</span>
                  <span className="text-muted-foreground">runs</span>
                </div>
                <div className="flex items-center gap-1">
                  <CheckCircle className="h-3 w-3 text-green-500" />
                  <span className="font-medium">
                    {(workflow.successRate * 100).toFixed(0)}%
                  </span>
                  <span className="text-muted-foreground">success</span>
                </div>
              </div>

              {workflow.avgExecutionTime && (
                <div className="flex items-center gap-1 text-sm">
                  <Clock className="h-3 w-3 text-muted-foreground" />
                  <span className="text-muted-foreground">Avg time:</span>
                  <span className="font-medium">
                    {workflow.avgExecutionTime}
                  </span>
                </div>
              )}

              <div className="flex flex-wrap gap-1">
                {workflow.metadata.tags.slice(0, 2).map((tag) => (
                  <Badge key={tag} variant="outline" className="text-xs">
                    {tag}
                  </Badge>
                ))}
                {workflow.metadata.tags.length > 2 && (
                  <Badge variant="outline" className="text-xs">
                    +{workflow.metadata.tags.length - 2}
                  </Badge>
                )}
              </div>

              <Separator />

              <div className="flex items-center justify-between text-xs text-muted-foreground">
                <div className="flex items-center gap-2">
                  <Avatar className="h-5 w-5">
                    <AvatarFallback className="text-[10px]">
                      {getInitials(
                        workflow.metadata.author || workflow.createdBy
                      )}
                    </AvatarFallback>
                  </Avatar>
                  <span>{workflow.metadata.author || workflow.createdBy}</span>
                </div>
                <span>{formatDate(workflow.updatedAt)}</span>
              </div>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )

  const renderListView = () => (
    <div className="overflow-x-auto rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-12">
              <Checkbox
                checked={
                  selectedWorkflows.size === paginatedWorkflows.length &&
                  paginatedWorkflows.length > 0
                }
                onCheckedChange={handleSelectAll}
              />
            </TableHead>
            <TableHead>
              <Button
                variant="ghost"
                size="sm"
                className="-ml-3 h-8 data-[state=open]:bg-accent"
                onClick={() => handleSort('name')}
              >
                Workflow Name
                {renderSortIcon('name')}
              </Button>
            </TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Category</TableHead>
            <TableHead>
              <Button
                variant="ghost"
                size="sm"
                className="-ml-3 h-8 data-[state=open]:bg-accent"
                onClick={() => handleSort('executionCount')}
              >
                Executions
                {renderSortIcon('executionCount')}
              </Button>
            </TableHead>
            <TableHead>
              <Button
                variant="ghost"
                size="sm"
                className="-ml-3 h-8 data-[state=open]:bg-accent"
                onClick={() => handleSort('successRate')}
              >
                Success Rate
                {renderSortIcon('successRate')}
              </Button>
            </TableHead>
            <TableHead>Author</TableHead>
            <TableHead>
              <Button
                variant="ghost"
                size="sm"
                className="-ml-3 h-8 data-[state=open]:bg-accent"
                onClick={() => handleSort('updatedAt')}
              >
                Last Updated
                {renderSortIcon('updatedAt')}
              </Button>
            </TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {paginatedWorkflows.length > 0 ? (
            paginatedWorkflows.map((workflow) => (
              <TableRow
                key={workflow.id}
                className={cn(
                  'cursor-pointer',
                  selectedWorkflows.has(workflow.id) && 'bg-muted/50'
                )}
                onClick={() =>
                  router.push(`/plugins/workflows/library/${workflow.id}`)
                }
              >
                <TableCell onClick={(e) => e.stopPropagation()}>
                  <Checkbox
                    checked={selectedWorkflows.has(workflow.id)}
                    onCheckedChange={(checked) =>
                      handleSelectWorkflow(workflow.id, checked as boolean)
                    }
                  />
                </TableCell>
                <TableCell>
                  <div className="flex flex-col">
                    <span className="font-medium">{workflow.name}</span>
                    <span className="max-w-[300px] truncate text-sm text-muted-foreground">
                      {workflow.description}
                    </span>
                  </div>
                </TableCell>
                <TableCell>
                  <Badge
                    className={statusColors[workflow.status]}
                    variant="secondary"
                  >
                    <span className="flex items-center gap-1">
                      {statusIcons[workflow.status]}
                      {workflow.status}
                    </span>
                  </Badge>
                </TableCell>
                <TableCell>
                  {workflow.metadata.category && (
                    <Badge variant="outline">
                      {workflow.metadata.category}
                    </Badge>
                  )}
                </TableCell>
                <TableCell>
                  <span className="font-medium">
                    {workflow.executionCount.toLocaleString()}
                  </span>
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-2">
                    <span className="font-medium">
                      {(workflow.successRate * 100).toFixed(1)}%
                    </span>
                    <div className="h-1 w-12 rounded bg-gray-200">
                      <div
                        className="h-1 rounded bg-green-500"
                        style={{ width: `${workflow.successRate * 100}%` }}
                      />
                    </div>
                  </div>
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-2">
                    <Avatar className="h-6 w-6">
                      <AvatarFallback className="text-[10px]">
                        {getInitials(
                          workflow.metadata.author || workflow.createdBy
                        )}
                      </AvatarFallback>
                    </Avatar>
                    <span className="text-sm">
                      {workflow.metadata.author || workflow.createdBy}
                    </span>
                  </div>
                </TableCell>
                <TableCell>{formatDate(workflow.updatedAt)}</TableCell>
                <TableCell
                  className="text-right"
                  onClick={(e) => e.stopPropagation()}
                >
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" className="h-8 w-8 p-0">
                        <span className="sr-only">Open menu</span>
                        <MoreHorizontal className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuLabel>Actions</DropdownMenuLabel>
                      <DropdownMenuItem
                        onClick={() =>
                          router.push(
                            `/plugins/workflows/library/${workflow.id}`
                          )
                        }
                      >
                        <Eye className="mr-2 h-4 w-4" />
                        View Details
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={() =>
                          router.push(
                            `/plugins/workflows/library/${workflow.id}/designer`
                          )
                        }
                      >
                        <Edit className="mr-2 h-4 w-4" />
                        Edit Workflow
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={() =>
                          router.push(
                            `/plugins/workflows/executions/start?workflowId=${workflow.id}`
                          )
                        }
                      >
                        <Play className="mr-2 h-4 w-4" />
                        Start Execution
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        onClick={() =>
                          console.log('Duplicating workflow:', workflow.id)
                        }
                      >
                        <Copy className="mr-2 h-4 w-4" />
                        Duplicate
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={() => {
                          setSelectedWorkflowForExport(workflow)
                          setShowExportDialog(true)
                        }}
                      >
                        <Download className="mr-2 h-4 w-4" />
                        Export
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={async () => {
                          if (
                            confirm(
                              'Are you sure you want to delete this workflow?'
                            )
                          ) {
                            const result = await deleteWorkflow(workflow.id)
                            if (result.success) {
                              toast({
                                title: 'Success',
                                description: 'Workflow deleted successfully'
                              })
                              loadWorkflows()
                            } else {
                              toast({
                                title: 'Error',
                                description: result.error,
                                variant: 'destructive'
                              })
                            }
                          }
                        }}
                        className="text-red-600"
                      >
                        <Trash2 className="mr-2 h-4 w-4" />
                        Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={9} className="h-24 text-center">
                No workflows found.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  )

  if (loading) {
    return (
      <div className="flex h-[60vh] items-center justify-center">
        <div className="flex flex-col items-center gap-2">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <p className="text-sm text-muted-foreground">Loading workflows...</p>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Workflow Library</h1>
          <p className="text-muted-foreground">
            Browse, manage, and organize your workflow definitions
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            onClick={() => setShowImportDialog(true)}
            variant="outline"
            className="flex items-center gap-2"
          >
            <Upload className="h-4 w-4" />
            <span>Import</span>
          </Button>
          <Button
            onClick={() => router.push('/plugins/workflows/library/create')}
            className="flex items-center gap-2"
          >
            <Plus className="h-4 w-4" />
            <span>Create Workflow</span>
          </Button>
        </div>
      </div>

      {/* Bulk Actions Bar */}
      {selectedWorkflows.size > 0 && (
        <div className="flex items-center justify-between rounded-lg bg-muted p-4">
          <span className="text-sm font-medium">
            {selectedWorkflows.size} workflow
            {selectedWorkflows.size > 1 ? 's' : ''} selected
          </span>
          <div className="flex items-center gap-2">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm">
                  <Power className="mr-2 h-4 w-4" />
                  Change Status
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent>
                <DropdownMenuItem
                  onClick={() => handleBulkStatusChange('ACTIVE')}
                >
                  <CheckCircle className="mr-2 h-4 w-4 text-green-500" />
                  Activate
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() => handleBulkStatusChange('INACTIVE')}
                >
                  <Clock className="mr-2 h-4 w-4 text-yellow-500" />
                  Deactivate
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() => handleBulkStatusChange('DEPRECATED')}
                >
                  <X className="mr-2 h-4 w-4 text-red-500" />
                  Deprecate
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <Button onClick={handleBulkExport} variant="outline" size="sm">
              <FileDown className="mr-2 h-4 w-4" />
              Export
            </Button>
            <Button
              onClick={handleBulkDelete}
              variant="outline"
              size="sm"
              className="text-red-600"
            >
              <Trash2 className="mr-2 h-4 w-4" />
              Delete
            </Button>
            <Button
              onClick={() => setSelectedWorkflows(new Set())}
              variant="ghost"
              size="sm"
            >
              Cancel
            </Button>
          </div>
        </div>
      )}

      {/* Filters and Search */}
      <div className="space-y-4">
        <div className="flex flex-col gap-4 sm:flex-row">
          <WorkflowAdvancedSearch
            onSearch={handleAdvancedSearch}
            workflows={workflows}
            className="flex-1"
          />

          <div className="flex items-center gap-2">
            <Select
              value={selectedStatus}
              onValueChange={(value: WorkflowStatus | 'ALL') =>
                setSelectedStatus(value)
              }
            >
              <SelectTrigger className="w-[140px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="ALL">All Status</SelectItem>
                <SelectItem value="ACTIVE">Active</SelectItem>
                <SelectItem value="INACTIVE">Inactive</SelectItem>
                <SelectItem value="DRAFT">Draft</SelectItem>
                <SelectItem value="DEPRECATED">Deprecated</SelectItem>
              </SelectContent>
            </Select>

            <Select
              value={selectedCategory}
              onValueChange={setSelectedCategory}
            >
              <SelectTrigger className="w-[160px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {workflowCategories.map((category) => (
                  <SelectItem key={category.value} value={category.value}>
                    {category.label}
                  </SelectItem>
                ))}
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

        {/* Advanced Filters */}
        <div className="flex flex-wrap items-center gap-2">
          <Select
            value={selectedFrequency}
            onValueChange={setSelectedFrequency}
          >
            <SelectTrigger className="w-[180px]">
              <Zap className="mr-2 h-4 w-4" />
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {executionFrequencies.map((freq) => (
                <SelectItem key={freq.value} value={freq.value}>
                  {freq.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select value={selectedAuthor} onValueChange={setSelectedAuthor}>
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="All Authors" />
            </SelectTrigger>
            <SelectContent>
              {authors.map((author) => (
                <SelectItem key={author} value={author}>
                  {author === 'ALL' ? 'All Authors' : author}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Popover>
            <PopoverTrigger asChild>
              <Button
                variant="outline"
                className={cn(
                  'w-[240px] justify-start text-left font-normal',
                  !dateRange.from && !dateRange.to && 'text-muted-foreground'
                )}
              >
                <CalendarIcon className="mr-2 h-4 w-4" />
                {dateRange.from ? (
                  dateRange.to ? (
                    <>
                      {format(dateRange.from, 'LLL dd, y')} -{' '}
                      {format(dateRange.to, 'LLL dd, y')}
                    </>
                  ) : (
                    format(dateRange.from, 'LLL dd, y')
                  )
                ) : (
                  <span>Created date range</span>
                )}
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-auto p-0" align="start">
              <Calendar
                initialFocus
                mode="range"
                defaultMonth={dateRange.from}
                selected={{ from: dateRange.from, to: dateRange.to }}
                onSelect={(range: any) =>
                  setDateRange(range || { from: undefined, to: undefined })
                }
                numberOfMonths={2}
              />
            </PopoverContent>
          </Popover>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="sm">
                <ArrowUpDown className="mr-2 h-4 w-4" />
                Sort: {sortField} ({sortOrder})
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              <DropdownMenuLabel>Sort by</DropdownMenuLabel>
              <DropdownMenuItem onClick={() => handleSort('name')}>
                Name
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => handleSort('createdAt')}>
                Created Date
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => handleSort('updatedAt')}>
                Last Modified
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => handleSort('executionCount')}>
                Execution Count
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => handleSort('successRate')}>
                Success Rate
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Results Summary */}
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          {filteredAndSortedWorkflows.length} workflow
          {filteredAndSortedWorkflows.length !== 1 ? 's' : ''} found
          {hasActiveFilters && ' (filtered)'}
        </p>
        {hasActiveFilters && (
          <Button
            variant="ghost"
            size="sm"
            onClick={clearFilters}
            className="flex items-center gap-2"
          >
            <Filter className="h-4 w-4" />
            <span>Clear Filters</span>
          </Button>
        )}
      </div>

      {/* Content */}
      {filteredAndSortedWorkflows.length === 0 ? (
        <Card>
          <CardContent className="py-16">
            <div className="text-center">
              <FileText className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
              <h3 className="mb-2 text-lg font-medium">No workflows found</h3>
              <p className="mb-4 text-muted-foreground">
                {hasActiveFilters
                  ? 'Try adjusting your search criteria or filters'
                  : 'Get started by creating your first workflow'}
              </p>
              {!hasActiveFilters && (
                <Button
                  onClick={() =>
                    router.push('/plugins/workflows/library/create')
                  }
                >
                  <Plus className="mr-2 h-4 w-4" />
                  Create Workflow
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

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <div className="text-sm text-muted-foreground">
            Showing {(currentPage - 1) * itemsPerPage + 1} to{' '}
            {Math.min(
              currentPage * itemsPerPage,
              filteredAndSortedWorkflows.length
            )}{' '}
            of {filteredAndSortedWorkflows.length} workflows
          </div>
          <div className="flex items-center gap-2">
            <Select
              value={itemsPerPage.toString()}
              onValueChange={(value) => {
                setItemsPerPage(parseInt(value))
                setCurrentPage(1)
              }}
            >
              <SelectTrigger className="w-[100px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="12">12 / page</SelectItem>
                <SelectItem value="24">24 / page</SelectItem>
                <SelectItem value="48">48 / page</SelectItem>
                <SelectItem value="96">96 / page</SelectItem>
              </SelectContent>
            </Select>

            <div className="flex items-center gap-1">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setCurrentPage(1)}
                disabled={currentPage === 1}
              >
                <ChevronsLeft className="h-4 w-4" />
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setCurrentPage(currentPage - 1)}
                disabled={currentPage === 1}
              >
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <span className="px-2 text-sm">
                Page {currentPage} of {totalPages}
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setCurrentPage(currentPage + 1)}
                disabled={currentPage === totalPages}
              >
                <ChevronRight className="h-4 w-4" />
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setCurrentPage(totalPages)}
                disabled={currentPage === totalPages}
              >
                <ChevronsRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Import Dialog */}
      <WorkflowImportExport
        open={showImportDialog}
        onOpenChange={setShowImportDialog}
        mode="import"
        onImport={(importedWorkflow) => {
          console.log('Imported workflow:', importedWorkflow)
          toast({
            title: 'Success',
            description: 'Workflow imported successfully'
          })
          loadWorkflows()
        }}
      />

      {/* Export Dialog */}
      <WorkflowImportExport
        workflow={selectedWorkflowForExport}
        open={showExportDialog}
        onOpenChange={setShowExportDialog}
        mode="export"
      />
    </div>
  )
}
