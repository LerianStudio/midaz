'use client'

import { useState, useMemo } from 'react'
import { ColumnDef } from '@tanstack/react-table'
import {
  MoreHorizontal,
  Search,
  Filter,
  External,
  Database,
  Eye,
  Edit,
  Copy,
  Trash2,
  Users,
  Activity,
  Download
} from 'lucide-react'
import Link from 'next/link'
import { formatDistanceToNow } from 'date-fns'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
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
import { EntityDataTable } from '@/components/entity-data-table'
import { AccountType } from '@/core/domain/mock-data/accounting-mock-data'

interface AccountTypeDataTableProps {
  data: AccountType[]
}

export function AccountTypeDataTable({ data }: AccountTypeDataTableProps) {
  const [searchQuery, setSearchQuery] = useState('')
  const [domainFilter, setDomainFilter] = useState<string>('all')
  const [statusFilter, setStatusFilter] = useState<string>('all')

  // Filter data based on search and filters
  const filteredData = useMemo(() => {
    return data.filter((accountType) => {
      const matchesSearch =
        accountType.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        accountType.keyValue
          .toLowerCase()
          .includes(searchQuery.toLowerCase()) ||
        accountType.description
          .toLowerCase()
          .includes(searchQuery.toLowerCase())

      const matchesDomain =
        domainFilter === 'all' || accountType.domain === domainFilter
      const matchesStatus =
        statusFilter === 'all' || accountType.status === statusFilter

      return matchesSearch && matchesDomain && matchesStatus
    })
  }, [data, searchQuery, domainFilter, statusFilter])

  const columns: ColumnDef<AccountType>[] = [
    {
      accessorKey: 'name',
      header: 'Account Type',
      cell: ({ row }) => {
        const accountType = row.original
        return (
          <div className="min-w-0">
            <div className="truncate font-medium text-gray-900">
              {accountType.name}
            </div>
            <div className="truncate text-sm text-gray-500">
              {accountType.description}
            </div>
          </div>
        )
      }
    },
    {
      accessorKey: 'keyValue',
      header: 'Key Value',
      cell: ({ row }) => (
        <code className="rounded bg-gray-100 px-2 py-1 font-mono text-sm">
          {row.getValue('keyValue')}
        </code>
      )
    },
    {
      accessorKey: 'domain',
      header: 'Domain',
      cell: ({ row }) => {
        const domain = row.getValue('domain') as string
        return (
          <Badge
            variant={domain === 'ledger' ? 'default' : 'secondary'}
            className="gap-1"
          >
            {domain === 'ledger' ? (
              <Database className="h-3 w-3" />
            ) : (
              <External className="h-3 w-3" />
            )}
            {domain === 'ledger' ? 'Ledger' : 'External'}
          </Badge>
        )
      }
    },
    {
      accessorKey: 'status',
      header: 'Status',
      cell: ({ row }) => {
        const status = row.getValue('status') as string
        const variants = {
          active: 'default',
          inactive: 'secondary',
          draft: 'outline',
          invalid: 'destructive'
        } as const

        return (
          <Badge
            variant={variants[status as keyof typeof variants] || 'secondary'}
          >
            {status}
          </Badge>
        )
      }
    },
    {
      accessorKey: 'usageCount',
      header: 'Usage',
      cell: ({ row }) => {
        const accountType = row.original
        return (
          <div className="text-center">
            <div className="font-medium">
              {accountType.usageCount.toLocaleString()}
            </div>
            <div className="flex items-center justify-center gap-1 text-xs text-gray-500">
              <Users className="h-3 w-3" />
              {accountType.linkedAccounts} accounts
            </div>
          </div>
        )
      }
    },
    {
      accessorKey: 'lastUsed',
      header: 'Last Used',
      cell: ({ row }) => {
        const lastUsed = row.getValue('lastUsed') as string
        return (
          <div className="text-sm text-gray-600">
            {formatDistanceToNow(new Date(lastUsed), { addSuffix: true })}
          </div>
        )
      }
    },
    {
      id: 'actions',
      header: '',
      cell: ({ row }) => {
        const accountType = row.original
        return (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              <DropdownMenuItem asChild>
                <Link
                  href={`/plugins/accounting/account-types/${accountType.id}`}
                >
                  <Eye className="mr-2 h-4 w-4" />
                  View Details
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link
                  href={`/plugins/accounting/account-types/${accountType.id}/analytics`}
                >
                  <Activity className="mr-2 h-4 w-4" />
                  Analytics
                </Link>
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem>
                <Edit className="mr-2 h-4 w-4" />
                Edit
              </DropdownMenuItem>
              <DropdownMenuItem>
                <Copy className="mr-2 h-4 w-4" />
                Duplicate
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem className="text-red-600">
                <Trash2 className="mr-2 h-4 w-4" />
                Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )
      }
    }
  ]

  return (
    <div className="space-y-4">
      {/* Search and Filters */}
      <div className="flex flex-col items-center justify-between gap-4 sm:flex-row">
        <div className="flex w-full flex-1 items-center gap-2 sm:w-auto">
          <div className="relative flex-1 sm:w-80 sm:flex-initial">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-gray-400" />
            <Input
              placeholder="Search account types..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-10"
            />
          </div>
          <Select value={domainFilter} onValueChange={setDomainFilter}>
            <SelectTrigger className="w-32">
              <Filter className="mr-2 h-4 w-4" />
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Domains</SelectItem>
              <SelectItem value="ledger">Ledger</SelectItem>
              <SelectItem value="external">External</SelectItem>
            </SelectContent>
          </Select>
          <Select value={statusFilter} onValueChange={setStatusFilter}>
            <SelectTrigger className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Status</SelectItem>
              <SelectItem value="active">Active</SelectItem>
              <SelectItem value="inactive">Inactive</SelectItem>
              <SelectItem value="draft">Draft</SelectItem>
              <SelectItem value="invalid">Invalid</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm">
            <Download className="mr-2 h-4 w-4" />
            Export
          </Button>
        </div>
      </div>

      {/* Stats Summary */}
      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <div className="rounded-lg border bg-white p-4">
          <div className="text-2xl font-bold text-blue-600">
            {filteredData.length}
          </div>
          <div className="text-sm text-gray-600">Total Types</div>
        </div>
        <div className="rounded-lg border bg-white p-4">
          <div className="text-2xl font-bold text-green-600">
            {filteredData.filter((t) => t.status === 'active').length}
          </div>
          <div className="text-sm text-gray-600">Active</div>
        </div>
        <div className="rounded-lg border bg-white p-4">
          <div className="text-2xl font-bold text-purple-600">
            {filteredData.filter((t) => t.domain === 'ledger').length}
          </div>
          <div className="text-sm text-gray-600">Ledger Domain</div>
        </div>
        <div className="rounded-lg border bg-white p-4">
          <div className="text-2xl font-bold text-orange-600">
            {filteredData
              .reduce((sum, t) => sum + t.usageCount, 0)
              .toLocaleString()}
          </div>
          <div className="text-sm text-gray-600">Total Usage</div>
        </div>
      </div>

      {/* Data Table */}
      <div className="rounded-lg border bg-white">
        <EntityDataTable
          columns={columns}
          data={filteredData}
          searchValue={searchQuery}
          onSearchChange={setSearchQuery}
          enableColumnVisibility
          enablePagination
          pageSize={10}
        />
      </div>
    </div>
  )
}
