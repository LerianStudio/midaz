'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Plus, Search, Filter, Download } from 'lucide-react'
import { useRouter } from 'next/navigation'
import { Input } from '@/components/ui/input'
import { PackageDataTable } from '@/components/fees/packages/package-data-table'
import {
  mockFeePackages,
  searchPackages
} from '@/components/fees/mock/fee-mock-data'
import { FeePackage } from '@/components/fees/types/fee-types'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'

export default function PackagesPage() {
  const intl = useIntl()
  const router = useRouter()
  const [packages, setPackages] = React.useState<FeePackage[]>(mockFeePackages)
  const [searchQuery, setSearchQuery] = React.useState('')
  const [statusFilter, setStatusFilter] = React.useState<
    'all' | 'active' | 'inactive'
  >('all')
  const [isLoading, setIsLoading] = React.useState(false)

  // Filter packages based on search and status
  React.useEffect(() => {
    let filtered = mockFeePackages

    // Apply search filter
    if (searchQuery) {
      filtered = searchPackages(searchQuery)
    }

    // Apply status filter
    if (statusFilter !== 'all') {
      filtered = filtered.filter((pkg) =>
        statusFilter === 'active' ? pkg.active : !pkg.active
      )
    }

    setPackages(filtered)
  }, [searchQuery, statusFilter])

  const handleSearch = (event: React.ChangeEvent<HTMLInputElement>) => {
    setSearchQuery(event.target.value)
  }

  const handleExport = () => {
    // Mock export functionality
    const csvContent = [
      ['ID', 'Name', 'Status', 'Created At', 'Waived Accounts'],
      ...packages.map((pkg) => [
        pkg.id,
        pkg.name,
        pkg.active ? 'Active' : 'Inactive',
        new Date(pkg.createdAt).toLocaleDateString(),
        pkg.waivedAccounts.length
      ])
    ]
      .map((row) => row.join(','))
      .join('\n')

    const blob = new Blob([csvContent], { type: 'text/csv' })
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'fee-packages.csv'
    a.click()
  }

  const handleDelete = async (id: string) => {
    // Mock delete - in real app would call API
    setIsLoading(true)
    setTimeout(() => {
      setPackages((prev) => prev.filter((pkg) => pkg.id !== id))
      setIsLoading(false)
    }, 500)
  }

  const handleToggleStatus = async (id: string) => {
    // Mock status toggle - in real app would call API
    setIsLoading(true)
    setTimeout(() => {
      setPackages((prev) =>
        prev.map((pkg) =>
          pkg.id === id ? { ...pkg, active: !pkg.active } : pkg
        )
      )
      setIsLoading(false)
    }, 500)
  }

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'fees.packages.title',
              defaultMessage: 'Fee Packages'
            })}
            subtitle={intl.formatMessage({
              id: 'fees.packages.subtitle',
              defaultMessage:
                'Manage and configure fee calculation rules for your organization'
            })}
          />
          <PageHeader.ActionButtons>
            <Button
              variant="outline"
              size="sm"
              onClick={handleExport}
              disabled={packages.length === 0}
            >
              <Download className="mr-2 h-4 w-4" />
              Export
            </Button>
            <Button
              size="sm"
              onClick={() => router.push('/plugins/fees/packages/create')}
            >
              <Plus className="mr-2 h-4 w-4" />
              {intl.formatMessage({
                id: 'fees.packages.createButton',
                defaultMessage: 'Create Package'
              })}
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>
      </PageHeader.Root>

      {/* Filters */}
      <div className="flex flex-col gap-4 sm:flex-row">
        <div className="relative max-w-md flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
          <Input
            placeholder={intl.formatMessage({
              id: 'fees.packages.searchPlaceholder',
              defaultMessage: 'Search packages...'
            })}
            value={searchQuery}
            onChange={handleSearch}
            className="pl-10"
          />
        </div>
        <Select
          value={statusFilter}
          onValueChange={(value: any) => setStatusFilter(value)}
        >
          <SelectTrigger className="w-[180px]">
            <Filter className="mr-2 h-4 w-4" />
            <SelectValue placeholder="Filter by status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Packages</SelectItem>
            <SelectItem value="active">Active Only</SelectItem>
            <SelectItem value="inactive">Inactive Only</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Data Table */}
      <PackageDataTable
        packages={packages}
        isLoading={isLoading}
        onEdit={(pkg) => router.push(`/plugins/fees/packages/${pkg.id}`)}
        onDelete={handleDelete}
        onToggleStatus={handleToggleStatus}
      />
    </div>
  )
}
