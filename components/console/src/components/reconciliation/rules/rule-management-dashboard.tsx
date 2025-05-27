'use client'

import { useState, useEffect } from 'react'
import {
  Plus,
  Search,
  Filter,
  Download,
  Play,
  Pause,
  Edit,
  Copy,
  Trash2,
  MoreHorizontal,
  ArrowUpDown,
  TrendingUp,
  Target,
  Zap,
  CheckCircle,
  AlertTriangle,
  Clock,
  Settings,
  BarChart3,
  Brain
} from 'lucide-react'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  mockReconciliationRules,
  ReconciliationRule
} from '@/lib/mock-data/reconciliation-unified'
import { VisualRuleBuilder } from './visual-rule-builder'

interface RuleManagementDashboardProps {
  className?: string
}

interface RuleFilters {
  status: 'all' | 'active' | 'inactive'
  ruleType:
    | 'all'
    | 'amount'
    | 'date'
    | 'string'
    | 'regex'
    | 'metadata'
    | 'composite'
  performance: 'all' | 'high' | 'medium' | 'low'
}

export function RuleManagementDashboard({
  className
}: RuleManagementDashboardProps) {
  const [rules, setRules] = useState<ReconciliationRule[]>(
    mockReconciliationRules
  )
  const [selectedRules, setSelectedRules] = useState<string[]>([])
  const [selectedRuleId, setSelectedRuleId] = useState<string | null>(null)
  const [showRuleBuilder, setShowRuleBuilder] = useState(false)
  const [filters, setFilters] = useState<RuleFilters>({
    status: 'all',
    ruleType: 'all',
    performance: 'all'
  })
  const [searchTerm, setSearchTerm] = useState('')
  const [sortField, setSortField] = useState<
    | 'priority'
    | 'performance.successRate'
    | 'performance.matchCount'
    | 'updatedAt'
  >('priority')
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc')

  // Filter rules based on current filters
  const filteredRules = rules.filter((rule) => {
    const statusMatch =
      filters.status === 'all' ||
      (filters.status === 'active' ? rule.isActive : !rule.isActive)
    const typeMatch =
      filters.ruleType === 'all' || rule.ruleType === filters.ruleType
    const performanceMatch =
      filters.performance === 'all' ||
      (filters.performance === 'high' && rule.performance.successRate >= 0.9) ||
      (filters.performance === 'medium' &&
        rule.performance.successRate >= 0.7 &&
        rule.performance.successRate < 0.9) ||
      (filters.performance === 'low' && rule.performance.successRate < 0.7)

    const searchMatch =
      searchTerm === '' ||
      rule.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      rule.description.toLowerCase().includes(searchTerm.toLowerCase())

    return statusMatch && typeMatch && performanceMatch && searchMatch
  })

  // Sort rules
  const sortedRules = [...filteredRules].sort((a, b) => {
    let aValue: any
    let bValue: any

    if (sortField.includes('.')) {
      const [obj, prop] = sortField.split('.')
      aValue = (a as any)[obj][prop]
      bValue = (b as any)[obj][prop]
    } else {
      aValue = (a as any)[sortField]
      bValue = (b as any)[sortField]
    }

    if (sortField === 'updatedAt') {
      aValue = new Date(aValue).getTime()
      bValue = new Date(bValue).getTime()
    }

    if (sortDirection === 'asc') {
      return aValue > bValue ? 1 : -1
    } else {
      return aValue < bValue ? 1 : -1
    }
  })

  const getRuleTypeBadge = (type: string) => {
    const typeConfig = {
      amount: {
        label: 'Amount',
        color: 'bg-green-50 text-green-700 border-green-200'
      },
      date: {
        label: 'Date',
        color: 'bg-blue-50 text-blue-700 border-blue-200'
      },
      string: {
        label: 'String',
        color: 'bg-purple-50 text-purple-700 border-purple-200'
      },
      regex: {
        label: 'Regex',
        color: 'bg-orange-50 text-orange-700 border-orange-200'
      },
      metadata: {
        label: 'Metadata',
        color: 'bg-gray-50 text-gray-700 border-gray-200'
      },
      composite: {
        label: 'Composite',
        color: 'bg-indigo-50 text-indigo-700 border-indigo-200'
      }
    }

    const config = typeConfig[type as keyof typeof typeConfig] || {
      label: type,
      color: 'bg-gray-50 text-gray-700 border-gray-200'
    }
    return (
      <Badge variant="outline" className={config.color}>
        {config.label}
      </Badge>
    )
  }

  const getPerformanceBadge = (successRate: number) => {
    if (successRate >= 0.9) {
      return (
        <Badge className="bg-green-500">
          Excellent ({Math.round(successRate * 100)}%)
        </Badge>
      )
    } else if (successRate >= 0.7) {
      return (
        <Badge className="bg-yellow-500">
          Good ({Math.round(successRate * 100)}%)
        </Badge>
      )
    } else {
      return (
        <Badge className="bg-red-500">
          Poor ({Math.round(successRate * 100)}%)
        </Badge>
      )
    }
  }

  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedRules(sortedRules.map((r) => r.id))
    } else {
      setSelectedRules([])
    }
  }

  const handleSelectRule = (ruleId: string, checked: boolean) => {
    if (checked) {
      setSelectedRules((prev) => [...prev, ruleId])
    } else {
      setSelectedRules((prev) => prev.filter((id) => id !== ruleId))
    }
  }

  const handleBulkAction = (
    action: 'activate' | 'deactivate' | 'delete' | 'priority'
  ) => {
    const updatedRules = rules
      .map((rule) => {
        if (selectedRules.includes(rule.id)) {
          switch (action) {
            case 'activate':
              return { ...rule, isActive: true }
            case 'deactivate':
              return { ...rule, isActive: false }
            case 'delete':
              return null // Will be filtered out
            default:
              return rule
          }
        }
        return rule
      })
      .filter(Boolean) as ReconciliationRule[]

    setRules(updatedRules)
    setSelectedRules([])
  }

  const handleToggleRule = (ruleId: string) => {
    const updatedRules = rules.map((rule) => {
      if (rule.id === ruleId) {
        return { ...rule, isActive: !rule.isActive }
      }
      return rule
    })
    setRules(updatedRules)
  }

  const handleEditRule = (ruleId: string) => {
    setSelectedRuleId(ruleId)
    setShowRuleBuilder(true)
  }

  const handleCreateRule = () => {
    setSelectedRuleId(null)
    setShowRuleBuilder(true)
  }

  const stats = {
    total: rules.length,
    active: rules.filter((r) => r.isActive).length,
    inactive: rules.filter((r) => !r.isActive).length,
    highPerformance: rules.filter((r) => r.performance.successRate >= 0.9)
      .length,
    totalMatches: rules.reduce((sum, r) => sum + r.performance.matchCount, 0),
    avgSuccessRate:
      rules.reduce((sum, r) => sum + r.performance.successRate, 0) /
      rules.length
  }

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Header with Stats */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <Target className="h-5 w-5 text-blue-600" />
                Rule Management
              </CardTitle>
              <CardDescription>
                Create, test, and manage reconciliation rules for automated
                matching
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Button onClick={handleCreateRule} className="gap-2">
                <Plus className="h-4 w-4" />
                Create Rule
              </Button>
              <Button variant="outline" size="sm">
                <Download className="mr-2 h-4 w-4" />
                Export
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {/* Stats Grid */}
          <div className="mb-6 grid grid-cols-2 gap-4 md:grid-cols-6">
            <div className="rounded-lg bg-blue-50 p-3 text-center">
              <div className="text-2xl font-bold text-blue-700">
                {stats.total}
              </div>
              <div className="text-xs text-blue-600">Total Rules</div>
            </div>
            <div className="rounded-lg bg-green-50 p-3 text-center">
              <div className="text-2xl font-bold text-green-700">
                {stats.active}
              </div>
              <div className="text-xs text-green-600">Active</div>
            </div>
            <div className="rounded-lg bg-gray-50 p-3 text-center">
              <div className="text-2xl font-bold text-gray-700">
                {stats.inactive}
              </div>
              <div className="text-xs text-gray-600">Inactive</div>
            </div>
            <div className="rounded-lg bg-emerald-50 p-3 text-center">
              <div className="text-2xl font-bold text-emerald-700">
                {stats.highPerformance}
              </div>
              <div className="text-xs text-emerald-600">High Performance</div>
            </div>
            <div className="rounded-lg bg-purple-50 p-3 text-center">
              <div className="text-2xl font-bold text-purple-700">
                {stats.totalMatches.toLocaleString()}
              </div>
              <div className="text-xs text-purple-600">Total Matches</div>
            </div>
            <div className="rounded-lg bg-indigo-50 p-3 text-center">
              <div className="text-2xl font-bold text-indigo-700">
                {Math.round(stats.avgSuccessRate * 100)}%
              </div>
              <div className="text-xs text-indigo-600">Avg Success Rate</div>
            </div>
          </div>

          {/* Filters and Search */}
          <div className="mb-6 flex flex-wrap items-center gap-4">
            <div className="max-w-sm flex-1">
              <Input
                placeholder="Search rules..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
              />
            </div>
            <Select
              value={filters.status}
              onValueChange={(value: any) =>
                setFilters((prev) => ({ ...prev, status: value }))
              }
            >
              <SelectTrigger className="w-40">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Rules</SelectItem>
                <SelectItem value="active">Active</SelectItem>
                <SelectItem value="inactive">Inactive</SelectItem>
              </SelectContent>
            </Select>
            <Select
              value={filters.ruleType}
              onValueChange={(value: any) =>
                setFilters((prev) => ({ ...prev, ruleType: value }))
              }
            >
              <SelectTrigger className="w-40">
                <SelectValue placeholder="Type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Types</SelectItem>
                <SelectItem value="amount">Amount</SelectItem>
                <SelectItem value="date">Date</SelectItem>
                <SelectItem value="string">String</SelectItem>
                <SelectItem value="regex">Regex</SelectItem>
                <SelectItem value="metadata">Metadata</SelectItem>
                <SelectItem value="composite">Composite</SelectItem>
              </SelectContent>
            </Select>
            <Select
              value={filters.performance}
              onValueChange={(value: any) =>
                setFilters((prev) => ({ ...prev, performance: value }))
              }
            >
              <SelectTrigger className="w-40">
                <SelectValue placeholder="Performance" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Performance</SelectItem>
                <SelectItem value="high">High (90%+)</SelectItem>
                <SelectItem value="medium">Medium (70-89%)</SelectItem>
                <SelectItem value="low">Low (&lt;70%)</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Bulk Actions */}
          {selectedRules.length > 0 && (
            <div className="mb-6 flex items-center gap-4 rounded-lg bg-blue-50 p-4">
              <span className="text-sm font-medium">
                {selectedRules.length} rule(s) selected
              </span>
              <div className="flex gap-2">
                <Button size="sm" onClick={() => handleBulkAction('activate')}>
                  <Play className="mr-1 h-4 w-4" />
                  Activate
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => handleBulkAction('deactivate')}
                >
                  <Pause className="mr-1 h-4 w-4" />
                  Deactivate
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => handleBulkAction('delete')}
                >
                  <Trash2 className="mr-1 h-4 w-4" />
                  Delete
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Rules Table */}
      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-12">
                  <Checkbox
                    checked={
                      selectedRules.length === sortedRules.length &&
                      sortedRules.length > 0
                    }
                    onCheckedChange={handleSelectAll}
                  />
                </TableHead>
                <TableHead>Rule Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead
                  className="cursor-pointer"
                  onClick={() => {
                    setSortField('priority')
                    setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
                  }}
                >
                  <div className="flex items-center gap-1">
                    Priority
                    <ArrowUpDown className="h-4 w-4" />
                  </div>
                </TableHead>
                <TableHead>Status</TableHead>
                <TableHead
                  className="cursor-pointer"
                  onClick={() => {
                    setSortField('performance.successRate')
                    setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
                  }}
                >
                  <div className="flex items-center gap-1">
                    Performance
                    <ArrowUpDown className="h-4 w-4" />
                  </div>
                </TableHead>
                <TableHead
                  className="cursor-pointer"
                  onClick={() => {
                    setSortField('performance.matchCount')
                    setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
                  }}
                >
                  <div className="flex items-center gap-1">
                    Matches
                    <ArrowUpDown className="h-4 w-4" />
                  </div>
                </TableHead>
                <TableHead>Execution Time</TableHead>
                <TableHead>Last Updated</TableHead>
                <TableHead>Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedRules.map((rule) => (
                <TableRow key={rule.id}>
                  <TableCell>
                    <Checkbox
                      checked={selectedRules.includes(rule.id)}
                      onCheckedChange={(checked) =>
                        handleSelectRule(rule.id, checked as boolean)
                      }
                    />
                  </TableCell>
                  <TableCell>
                    <div className="space-y-1">
                      <div className="font-medium">{rule.name}</div>
                      <div className="max-w-[200px] truncate text-sm text-muted-foreground">
                        {rule.description}
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>{getRuleTypeBadge(rule.ruleType)}</TableCell>
                  <TableCell>
                    <Badge variant="outline" className="font-mono">
                      {rule.priority}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => handleToggleRule(rule.id)}
                        className="h-6 w-6 p-0"
                      >
                        {rule.isActive ? (
                          <CheckCircle className="h-4 w-4 text-green-600" />
                        ) : (
                          <Pause className="h-4 w-4 text-gray-400" />
                        )}
                      </Button>
                      <span className="text-sm">
                        {rule.isActive ? 'Active' : 'Inactive'}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="space-y-1">
                      {getPerformanceBadge(rule.performance.successRate)}
                      <div className="text-xs text-muted-foreground">
                        Avg confidence:{' '}
                        {Math.round(rule.performance.averageConfidence * 100)}%
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="text-sm font-medium">
                      {rule.performance.matchCount.toLocaleString()}
                    </div>
                  </TableCell>
                  <TableCell>
                    <span className="font-mono text-sm">
                      {rule.performance.executionTime}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="text-sm text-muted-foreground">
                      {new Date(rule.updatedAt).toLocaleDateString()}
                    </span>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => handleEditRule(rule.id)}
                      >
                        <Edit className="h-4 w-4" />
                      </Button>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button size="sm" variant="outline">
                            <MoreHorizontal className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent>
                          <DropdownMenuItem>
                            <Copy className="mr-2 h-4 w-4" />
                            Duplicate
                          </DropdownMenuItem>
                          <DropdownMenuItem>
                            <Play className="mr-2 h-4 w-4" />
                            Test Rule
                          </DropdownMenuItem>
                          <DropdownMenuItem>
                            <BarChart3 className="mr-2 h-4 w-4" />
                            View Analytics
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem className="text-red-600">
                            <Trash2 className="mr-2 h-4 w-4" />
                            Delete
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {/* Rule Builder Dialog */}
      <Dialog open={showRuleBuilder} onOpenChange={setShowRuleBuilder}>
        <DialogContent className="max-h-[90vh] max-w-6xl overflow-auto">
          <DialogHeader>
            <DialogTitle>
              {selectedRuleId ? 'Edit Rule' : 'Create New Rule'}
            </DialogTitle>
            <DialogDescription>
              {selectedRuleId
                ? 'Modify existing reconciliation rule'
                : 'Create a new reconciliation rule with visual builder'}
            </DialogDescription>
          </DialogHeader>
          <VisualRuleBuilder
            ruleId={selectedRuleId || undefined}
            onSave={(rule) => {
              console.log('Rule saved:', rule)
              setShowRuleBuilder(false)
              // Update rules list
            }}
            onTest={async (rule) => {
              // Mock test implementation
              await new Promise((resolve) => setTimeout(resolve, 2000))
              return {
                success: true,
                matches: Math.floor(Math.random() * 100) + 50,
                executionTime: `${Math.floor(Math.random() * 200) + 50}ms`,
                sampleMatches: [],
                errors: [],
                warnings: []
              }
            }}
          />
        </DialogContent>
      </Dialog>
    </div>
  )
}
