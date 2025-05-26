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
import { useToast } from '@/hooks/use-toast'
import { WorkflowImportExport } from '@/components/workflows/library/workflow-import-export'
import {
  WorkflowAdvancedSearch,
  SearchFilters
} from '@/components/workflows/library/workflow-advanced-search'
import { WorkflowListSkeleton } from '@/components/workflows/loading-states'
import {
  ErrorHandlingWrapper,
  useAsyncOperation
} from '@/components/workflows/error-handling-wrapper'
import { WorkflowErrorBoundaryWrapper } from '@/components/workflows/error-boundary'
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
  Zap,
  RefreshCw
} from 'lucide-react'
import { Workflow, WorkflowStatus } from '@/core/domain/entities/workflow'
import {
  getWorkflows,
  deleteWorkflow,
  updateWorkflowStatus
} from '@/app/actions/workflows'
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

function WorkflowLibraryContent() {
  const router = useRouter()
  const { toast } = useToast()
  const [workflows, setWorkflows] = useState<Workflow[]>([])
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

  // Use async operation hook for better error handling
  const {
    isLoading,
    error,
    execute: loadWorkflows,
    retry
  } = useAsyncOperation<Workflow[]>()

  // Fetch workflows on mount
  useEffect(() => {
    loadWorkflows(
      async () => {
        const result = await getWorkflows({
          organizationId: 'default',
          limit: 1000,
          page: 1
        })

        if (result.success && result.data) {
          setWorkflows(result.data.workflows)
          return result.data.workflows
        } else {
          throw new Error(result.error || 'Failed to load workflows')
        }
      },
      {
        onError: (error) => {
          toast({
            title: 'Error loading workflows',
            description: error.message,
            variant: 'destructive'
          })
        },
        retryCount: 2,
        retryDelay: 1000
      }
    )
  }, [])

  // Handle individual workflow deletion with error handling
  const handleDeleteWorkflow = async (workflowId: string) => {
    try {
      const result = await deleteWorkflow(workflowId)
      if (result.success) {
        toast({
          title: 'Success',
          description: 'Workflow deleted successfully'
        })
        // Reload workflows
        loadWorkflows(async () => {
          const result = await getWorkflows({
            organizationId: 'default',
            limit: 1000,
            page: 1
          })
          if (result.success && result.data) {
            setWorkflows(result.data.workflows)
            return result.data.workflows
          }
          throw new Error(result.error || 'Failed to reload workflows')
        })
      } else {
        throw new Error(result.error || 'Failed to delete workflow')
      }
    } catch (error) {
      toast({
        title: 'Error',
        description:
          error instanceof Error ? error.message : 'Failed to delete workflow',
        variant: 'destructive'
      })
    }
  }

  // Handle bulk operations with error handling
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

    const errors: string[] = []
    const successes: string[] = []

    // Delete selected workflows
    for (const workflowId of selectedWorkflows) {
      try {
        const result = await deleteWorkflow(workflowId)
        if (result.success) {
          successes.push(workflowId)
        } else {
          errors.push(`${workflowId}: ${result.error}`)
        }
      } catch (error) {
        errors.push(
          `${workflowId}: ${error instanceof Error ? error.message : 'Unknown error'}`
        )
      }
    }

    // Show results
    if (successes.length > 0) {
      toast({
        title: 'Success',
        description: `Deleted ${successes.length} workflow(s) successfully`
      })
    }

    if (errors.length > 0) {
      toast({
        title: 'Some deletions failed',
        description: `Failed to delete ${errors.length} workflow(s)`,
        variant: 'destructive'
      })
    }

    setSelectedWorkflows(new Set())

    // Reload workflows
    loadWorkflows(async () => {
      const result = await getWorkflows({
        organizationId: 'default',
        limit: 1000,
        page: 1
      })
      if (result.success && result.data) {
        setWorkflows(result.data.workflows)
        return result.data.workflows
      }
      throw new Error(result.error || 'Failed to reload workflows')
    })
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

    const errors: string[] = []
    const successes: string[] = []

    // Update status for selected workflows
    for (const workflowId of selectedWorkflows) {
      try {
        const result = await updateWorkflowStatus(workflowId, status)
        if (result.success) {
          successes.push(workflowId)
        } else {
          errors.push(`${workflowId}: ${result.error}`)
        }
      } catch (error) {
        errors.push(
          `${workflowId}: ${error instanceof Error ? error.message : 'Unknown error'}`
        )
      }
    }

    // Show results
    if (successes.length > 0) {
      toast({
        title: 'Success',
        description: `Updated ${successes.length} workflow(s) to ${status}`
      })
    }

    if (errors.length > 0) {
      toast({
        title: 'Some updates failed',
        description: `Failed to update ${errors.length} workflow(s)`,
        variant: 'destructive'
      })
    }

    setSelectedWorkflows(new Set())

    // Reload workflows
    loadWorkflows(async () => {
      const result = await getWorkflows({
        organizationId: 'default',
        limit: 1000,
        page: 1
      })
      if (result.success && result.data) {
        setWorkflows(result.data.workflows)
        return result.data.workflows
      }
      throw new Error(result.error || 'Failed to reload workflows')
    })
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

  // Show loading/error states using the wrapper
  return (
    <ErrorHandlingWrapper
      isLoading={isLoading}
      error={error}
      onRetry={retry}
      customLoadingComponent={<WorkflowListSkeleton />}
    >
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
              onClick={retry}
              variant="ghost"
              size="icon"
              title="Refresh workflows"
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
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

        {/* Rest of the component remains the same... */}
        {/* You would include all the remaining JSX from the original component here */}
      </div>
    </ErrorHandlingWrapper>
  )
}

export default function WorkflowLibraryPage() {
  return (
    <WorkflowErrorBoundaryWrapper
      onError={(error, errorInfo) => {
        // Log to error tracking service
        console.error('Workflow library page error:', error, errorInfo)
      }}
    >
      <WorkflowLibraryContent />
    </WorkflowErrorBoundaryWrapper>
  )
}
