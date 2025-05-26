'use client'

import { useState } from 'react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { AuditTrailTable } from '@/components/accounting/compliance/audit-trail-table'
import {
  Download,
  Filter,
  Search,
  Calendar,
  User,
  FileText,
  RefreshCw
} from 'lucide-react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { DatePickerWithRange } from '@/components/ui/date-picker-with-range'
import { DateRange } from 'react-day-picker'

// Mock audit trail data
const mockAuditEvents = [
  {
    id: 'audit-001',
    timestamp: '2024-12-30T14:20:00Z',
    event: 'account_type_created',
    entityType: 'account_type',
    entityId: 'at-007',
    entityName: 'Corporate Credit Account',
    userId: 'user-001',
    userName: 'John Smith',
    userEmail: 'john.smith@company.com',
    action: 'CREATE',
    description: 'Created new account type for corporate credit accounts',
    metadata: {
      keyValue: 'CORP_CREDIT',
      domain: 'ledger',
      category: 'asset'
    },
    ipAddress: '192.168.1.100',
    userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36',
    status: 'success',
    changes: {
      before: null,
      after: {
        name: 'Corporate Credit Account',
        code: 'CORP_CREDIT',
        domain: 'ledger'
      }
    }
  },
  {
    id: 'audit-002',
    timestamp: '2024-12-30T13:15:00Z',
    event: 'transaction_route_updated',
    entityType: 'transaction_route',
    entityId: 'tr-002',
    entityName: 'Merchant Payment Route',
    userId: 'user-002',
    userName: 'Sarah Johnson',
    userEmail: 'sarah.johnson@company.com',
    action: 'UPDATE',
    description: 'Updated fee percentage for merchant payment route',
    metadata: {
      previousFeePercentage: 2.9,
      newFeePercentage: 3.2,
      reason: 'Market adjustment'
    },
    ipAddress: '192.168.1.101',
    userAgent:
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36',
    status: 'success',
    changes: {
      before: { feePercentage: 2.9 },
      after: { feePercentage: 3.2 }
    }
  },
  {
    id: 'audit-003',
    timestamp: '2024-12-30T12:45:00Z',
    event: 'validation_rule_failed',
    entityType: 'account_type',
    entityId: 'at-008',
    entityName: 'Duplicate Test Account',
    userId: 'user-001',
    userName: 'John Smith',
    userEmail: 'john.smith@company.com',
    action: 'CREATE_FAILED',
    description: 'Account type creation failed due to duplicate key value',
    metadata: {
      keyValue: 'CHCK',
      validationError: 'Key value already exists',
      attemptedAction: 'create_account_type'
    },
    ipAddress: '192.168.1.100',
    userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36',
    status: 'failed',
    changes: {
      before: null,
      after: null
    }
  },
  {
    id: 'audit-004',
    timestamp: '2024-12-30T11:30:00Z',
    event: 'operation_route_created',
    entityType: 'operation_route',
    entityId: 'or-009',
    entityName: 'Credit Card Payment Operation',
    userId: 'user-003',
    userName: 'Mike Wilson',
    userEmail: 'mike.wilson@company.com',
    action: 'CREATE',
    description: 'Created new operation route for credit card payments',
    metadata: {
      transactionRouteId: 'tr-004',
      operationType: 'debit',
      sourceAccountType: 'CREDIT_CARD',
      destinationAccountType: 'MERCHANT'
    },
    ipAddress: '192.168.1.102',
    userAgent: 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36',
    status: 'success',
    changes: {
      before: null,
      after: {
        operationType: 'debit',
        sourceAccountTypeId: 'at-009',
        destinationAccountTypeId: 'at-003'
      }
    }
  },
  {
    id: 'audit-005',
    timestamp: '2024-12-30T10:15:00Z',
    event: 'compliance_check_completed',
    entityType: 'system',
    entityId: 'compliance-check-001',
    entityName: 'Daily Compliance Scan',
    userId: 'system',
    userName: 'System',
    userEmail: 'system@company.com',
    action: 'SYSTEM_CHECK',
    description: 'Automated daily compliance check completed successfully',
    metadata: {
      totalRulesChecked: 24,
      rulesPasssed: 23,
      rulesFailed: 1,
      complianceScore: 95.8,
      duration: '2.3s'
    },
    ipAddress: '127.0.0.1',
    userAgent: 'Midaz-Compliance-Scanner/1.0',
    status: 'success',
    changes: {
      before: { complianceScore: 94.2 },
      after: { complianceScore: 95.8 }
    }
  },
  {
    id: 'audit-006',
    timestamp: '2024-12-29T16:45:00Z',
    event: 'account_type_deleted',
    entityType: 'account_type',
    entityId: 'at-006-deleted',
    entityName: 'Deprecated Test Account',
    userId: 'user-002',
    userName: 'Sarah Johnson',
    userEmail: 'sarah.johnson@company.com',
    action: 'DELETE',
    description: 'Deleted deprecated test account type',
    metadata: {
      reason: 'No longer needed for testing',
      confirmationRequired: true,
      linkedAccounts: 0
    },
    ipAddress: '192.168.1.101',
    userAgent:
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36',
    status: 'success',
    changes: {
      before: {
        name: 'Deprecated Test Account',
        code: 'TEST_DEPRECATED',
        status: 'inactive'
      },
      after: null
    }
  }
]

export default function AuditTrailPage() {
  const [searchQuery, setSearchQuery] = useState('')
  const [dateRange, setDateRange] = useState<DateRange | undefined>()
  const [selectedUser, setSelectedUser] = useState<string>('')
  const [selectedAction, setSelectedAction] = useState<string>('')
  const [selectedStatus, setSelectedStatus] = useState<string>('')
  const [isRefreshing, setIsRefreshing] = useState(false)

  const handleRefresh = async () => {
    setIsRefreshing(true)
    // Simulate API call
    await new Promise((resolve) => setTimeout(resolve, 1000))
    setIsRefreshing(false)
  }

  const handleExport = () => {
    // Export logic would go here
    console.log('Exporting audit trail...')
  }

  const clearFilters = () => {
    setSearchQuery('')
    setDateRange(undefined)
    setSelectedUser('')
    setSelectedAction('')
    setSelectedStatus('')
  }

  // Filter the audit events based on current filters
  const filteredEvents = mockAuditEvents.filter((event) => {
    if (
      searchQuery &&
      !event.description.toLowerCase().includes(searchQuery.toLowerCase()) &&
      !event.entityName.toLowerCase().includes(searchQuery.toLowerCase()) &&
      !event.userName.toLowerCase().includes(searchQuery.toLowerCase())
    ) {
      return false
    }

    if (selectedUser && event.userId !== selectedUser) {
      return false
    }

    if (selectedAction && event.action !== selectedAction) {
      return false
    }

    if (selectedStatus && event.status !== selectedStatus) {
      return false
    }

    // Date range filtering would be implemented here

    return true
  })

  const uniqueUsers = Array.from(
    new Set(mockAuditEvents.map((e) => ({ id: e.userId, name: e.userName })))
  )
  const uniqueActions = Array.from(
    new Set(mockAuditEvents.map((e) => e.action))
  )
  const uniqueStatuses = Array.from(
    new Set(mockAuditEvents.map((e) => e.status))
  )

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Audit Trail</h1>
          <p className="text-muted-foreground">
            Complete audit trail of all accounting operations and system changes
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleRefresh}
            disabled={isRefreshing}
          >
            <RefreshCw
              className={`mr-2 h-4 w-4 ${isRefreshing ? 'animate-spin' : ''}`}
            />
            Refresh
          </Button>
          <Button variant="outline" size="sm" onClick={handleExport}>
            <Download className="mr-2 h-4 w-4" />
            Export
          </Button>
        </div>
      </div>

      {/* Filters */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Filter className="h-5 w-5" />
            Filters
          </CardTitle>
          <CardDescription>
            Filter audit events by date, user, action, or search terms
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
            {/* Search */}
            <div className="relative xl:col-span-2">
              <Search className="absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search events..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9"
              />
            </div>

            {/* Date Range */}
            <div>
              <DatePickerWithRange
                date={dateRange}
                onDateChange={setDateRange}
              />
            </div>

            {/* User Filter */}
            <div>
              <Select value={selectedUser} onValueChange={setSelectedUser}>
                <SelectTrigger>
                  <SelectValue placeholder="All users" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="">All users</SelectItem>
                  {uniqueUsers.map((user) => (
                    <SelectItem key={user.id} value={user.id}>
                      {user.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Action Filter */}
            <div>
              <Select value={selectedAction} onValueChange={setSelectedAction}>
                <SelectTrigger>
                  <SelectValue placeholder="All actions" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="">All actions</SelectItem>
                  {uniqueActions.map((action) => (
                    <SelectItem key={action} value={action}>
                      {action}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Status Filter */}
            <div>
              <Select value={selectedStatus} onValueChange={setSelectedStatus}>
                <SelectTrigger>
                  <SelectValue placeholder="All statuses" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="">All statuses</SelectItem>
                  {uniqueStatuses.map((status) => (
                    <SelectItem key={status} value={status}>
                      {status}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          {/* Active Filters Display */}
          {(searchQuery ||
            selectedUser ||
            selectedAction ||
            selectedStatus ||
            dateRange) && (
            <div className="mt-4 flex items-center gap-2">
              <span className="text-sm text-muted-foreground">
                Active filters:
              </span>
              {searchQuery && (
                <Badge variant="secondary">Search: {searchQuery}</Badge>
              )}
              {selectedUser && (
                <Badge variant="secondary">
                  User: {uniqueUsers.find((u) => u.id === selectedUser)?.name}
                </Badge>
              )}
              {selectedAction && (
                <Badge variant="secondary">Action: {selectedAction}</Badge>
              )}
              {selectedStatus && (
                <Badge variant="secondary">Status: {selectedStatus}</Badge>
              )}
              {dateRange && (
                <Badge variant="secondary">Date range applied</Badge>
              )}
              <Button variant="ghost" size="sm" onClick={clearFilters}>
                Clear all
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Summary Stats */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">{filteredEvents.length}</div>
            <p className="text-xs text-muted-foreground">Total Events</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">
              {filteredEvents.filter((e) => e.status === 'success').length}
            </div>
            <p className="text-xs text-muted-foreground">
              Successful Operations
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">
              {filteredEvents.filter((e) => e.status === 'failed').length}
            </div>
            <p className="text-xs text-muted-foreground">Failed Operations</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">
              {new Set(filteredEvents.map((e) => e.userId)).size}
            </div>
            <p className="text-xs text-muted-foreground">Unique Users</p>
          </CardContent>
        </Card>
      </div>

      {/* Audit Trail Table */}
      <AuditTrailTable events={filteredEvents} />
    </div>
  )
}
