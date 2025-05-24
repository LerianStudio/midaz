'use client'

import React, { useState } from 'react'
import { useIntl } from 'react-intl'
import { useRouter } from 'next/navigation'
import { PageHeader } from '@/components/page-header'
import { Breadcrumb } from '@/components/breadcrumb'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import {
  CreditCard,
  Plus,
  Search,
  Filter,
  MoreHorizontal,
  Edit3,
  Trash2,
  ExternalLink,
  Users,
  Building2,
  Banknote
} from 'lucide-react'
import {
  generateMockCustomers,
  generateMockAliases
} from '@/components/crm/customers/customer-mock-data'
import {
  Alias,
  Customer,
  CustomerType
} from '@/components/crm/customers/customer-types'

const AliasesPage = () => {
  const intl = useIntl()
  const router = useRouter()
  const { currentOrganization } = useOrganization()
  const [searchTerm, setSearchTerm] = useState('')

  const breadcrumbPaths = getBreadcrumbPaths([
    {
      name: currentOrganization.legalName
    },
    {
      name: intl.formatMessage({
        id: 'plugins.title',
        defaultMessage: 'Native Plugins'
      }),
      href: '/plugins'
    },
    {
      name: intl.formatMessage({
        id: 'crm.title',
        defaultMessage: 'CRM'
      }),
      href: '/plugins/crm'
    },
    {
      name: intl.formatMessage({
        id: 'crm.aliases',
        defaultMessage: 'Banking Aliases'
      })
    }
  ])

  // Generate mock data
  const customers = generateMockCustomers(50)
  const aliases = generateMockAliases(150)

  // Create customer lookup map
  const customerMap = customers.reduce(
    (acc, customer) => {
      acc[customer.id] = customer
      return acc
    },
    {} as Record<string, Customer>
  )

  // Filter aliases based on search
  const filteredAliases = aliases.filter((alias) => {
    const customer = customerMap[alias.holderId]
    return (
      alias.id.toLowerCase().includes(searchTerm.toLowerCase()) ||
      alias.bankingDetails.account
        .toLowerCase()
        .includes(searchTerm.toLowerCase()) ||
      alias.bankingDetails.bankId
        .toLowerCase()
        .includes(searchTerm.toLowerCase()) ||
      customer?.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      customer?.document.includes(searchTerm)
    )
  })

  const handleEditAlias = (alias: Alias) => {
    console.log('Edit alias:', alias)
    // TODO: Open edit alias dialog
  }

  const handleDeleteAlias = (alias: Alias) => {
    console.log('Delete alias:', alias)
    // TODO: Show confirmation dialog and delete
  }

  const handleViewCustomer = (customerId: string) => {
    router.push(`/plugins/crm/customers/${customerId}`)
  }

  const handleViewAccount = (alias: Alias) => {
    console.log('View account:', alias.accountId)
    // TODO: Navigate to account detail page
  }

  const getBankStatusBadge = (status: string) => {
    switch (status.toLowerCase()) {
      case 'active':
        return (
          <Badge variant="default" className="bg-green-100 text-green-800">
            Active
          </Badge>
        )
      case 'inactive':
        return <Badge variant="secondary">Inactive</Badge>
      case 'pending':
        return (
          <Badge variant="outline" className="bg-yellow-100 text-yellow-800">
            Pending
          </Badge>
        )
      default:
        return <Badge variant="secondary">{status}</Badge>
    }
  }

  const getAccountTypeBadge = (type: string) => {
    switch (type.toLowerCase()) {
      case 'checking':
        return (
          <Badge variant="outline" className="bg-blue-100 text-blue-800">
            Checking
          </Badge>
        )
      case 'savings':
        return (
          <Badge variant="outline" className="bg-green-100 text-green-800">
            Savings
          </Badge>
        )
      case 'business':
        return (
          <Badge variant="outline" className="bg-purple-100 text-purple-800">
            Business
          </Badge>
        )
      default:
        return <Badge variant="outline">{type}</Badge>
    }
  }

  const getCustomerTypeIcon = (type: CustomerType) => {
    return type === CustomerType.NATURAL_PERSON ? (
      <Users className="h-4 w-4 text-blue-600" />
    ) : (
      <Building2 className="h-4 w-4 text-purple-600" />
    )
  }

  // Calculate metrics
  const metrics = {
    total: aliases.length,
    active: aliases.filter((a) => a.status === 'active').length,
    checking: aliases.filter((a) => a.bankingDetails.type === 'CHECKING')
      .length,
    savings: aliases.filter((a) => a.bankingDetails.type === 'SAVINGS').length
  }

  return (
    <React.Fragment>
      <Breadcrumb paths={breadcrumbPaths} />

      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'crm.aliases.title',
              defaultMessage: 'Banking Aliases'
            })}
            subtitle={intl.formatMessage({
              id: 'crm.aliases.subtitle',
              defaultMessage:
                'Manage customer-to-account relationships and banking connections across all customers.'
            })}
          />
          <PageHeader.ActionButtons>
            <PageHeader.CollapsibleInfoTrigger
              question={intl.formatMessage({
                id: 'crm.aliases.helperTrigger.question',
                defaultMessage: 'What are Banking Aliases?'
              })}
            />

            <Button>
              <Plus className="mr-2 h-4 w-4" />
              {intl.formatMessage({
                id: 'crm.aliases.newAlias',
                defaultMessage: 'New Alias'
              })}
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>

        <PageHeader.CollapsibleInfo
          question={intl.formatMessage({
            id: 'crm.aliases.helperTrigger.question',
            defaultMessage: 'What are Banking Aliases?'
          })}
          answer={intl.formatMessage({
            id: 'crm.aliases.helperTrigger.answer',
            defaultMessage:
              'Banking aliases are connections between customer profiles and ledger accounts. They contain banking details like account numbers, routing information, and account types, enabling customers to be linked to multiple accounts across different ledgers.'
          })}
          seeMore={intl.formatMessage({
            id: 'common.read.docs',
            defaultMessage: 'Read the docs'
          })}
          href="https://docs.lerian.studio/docs/plugins/crm/aliases"
        />
      </PageHeader.Root>

      {/* Metrics Cards */}
      <div className="mt-8 grid grid-cols-1 gap-4 md:grid-cols-4">
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center space-x-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/10">
                <CreditCard className="h-4 w-4 text-primary" />
              </div>
              <div>
                <div className="text-2xl font-bold">{metrics.total}</div>
                <p className="text-xs text-muted-foreground">Total Aliases</p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center space-x-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-green-100 dark:bg-green-900">
                <Banknote className="h-4 w-4 text-green-600" />
              </div>
              <div>
                <div className="text-2xl font-bold">{metrics.active}</div>
                <p className="text-xs text-muted-foreground">Active Aliases</p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center space-x-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-blue-100 dark:bg-blue-900">
                <CreditCard className="h-4 w-4 text-blue-600" />
              </div>
              <div>
                <div className="text-2xl font-bold">{metrics.checking}</div>
                <p className="text-xs text-muted-foreground">
                  Checking Accounts
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center space-x-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-orange-100 dark:bg-orange-900">
                <Banknote className="h-4 w-4 text-orange-600" />
              </div>
              <div>
                <div className="text-2xl font-bold">{metrics.savings}</div>
                <p className="text-xs text-muted-foreground">
                  Savings Accounts
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Search and Filters */}
      <div className="mt-6 flex items-center space-x-4">
        <div className="relative max-w-sm flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={intl.formatMessage({
              id: 'crm.aliases.search.placeholder',
              defaultMessage: 'Search by customer, alias ID, or account...'
            })}
            value={searchTerm}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
              setSearchTerm(e.target.value)
            }
            className="pl-10"
          />
        </div>
        <Button variant="outline">
          <Filter className="mr-2 h-4 w-4" />
          {intl.formatMessage({
            id: 'common.filters',
            defaultMessage: 'Filters'
          })}
        </Button>
      </div>

      {/* Aliases Table */}
      <Card className="mt-6">
        <CardHeader>
          <CardTitle className="flex items-center space-x-2">
            <CreditCard className="h-5 w-5" />
            <span>Banking Aliases</span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {filteredAliases.length === 0 ? (
            <div className="py-8 text-center">
              <CreditCard className="mx-auto h-12 w-12 text-muted-foreground/50" />
              <h3 className="mt-2 text-sm font-semibold">No aliases found</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                {searchTerm
                  ? 'Try adjusting your search criteria.'
                  : 'Create the first banking alias.'}
              </p>
              {!searchTerm && (
                <div className="mt-6">
                  <Button>
                    <Plus className="mr-2 h-4 w-4" />
                    Create Alias
                  </Button>
                </div>
              )}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Customer</TableHead>
                  <TableHead>Alias ID</TableHead>
                  <TableHead>Bank Details</TableHead>
                  <TableHead>Account Type</TableHead>
                  <TableHead>Ledger</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredAliases.map((alias) => {
                  const customer = customerMap[alias.holderId]
                  return (
                    <TableRow key={alias.id}>
                      <TableCell>
                        {customer && (
                          <div className="flex items-center space-x-3">
                            <div className="flex-shrink-0">
                              {getCustomerTypeIcon(customer.type)}
                            </div>
                            <div>
                              <button
                                onClick={() => handleViewCustomer(customer.id)}
                                className="text-left font-medium hover:underline"
                              >
                                {customer.name}
                              </button>
                              <div className="text-sm text-muted-foreground">
                                {customer.document}
                              </div>
                            </div>
                          </div>
                        )}
                      </TableCell>
                      <TableCell className="font-mono text-sm">
                        {alias.id.slice(-8)}
                      </TableCell>
                      <TableCell>
                        <div className="space-y-1">
                          <div className="flex items-center space-x-2">
                            <span className="font-medium">
                              Bank {alias.bankingDetails.bankId}
                            </span>
                            <span className="text-muted-foreground">•</span>
                            <span className="text-muted-foreground">
                              Branch {alias.bankingDetails.branch}
                            </span>
                          </div>
                          <div className="text-sm text-muted-foreground">
                            Account: {alias.bankingDetails.account}
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        {getAccountTypeBadge(alias.bankingDetails.type)}
                      </TableCell>
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleViewAccount(alias)}
                          className="h-auto p-0 font-mono text-xs"
                        >
                          {alias.ledgerId.slice(-8)}
                          <ExternalLink className="ml-1 h-3 w-3" />
                        </Button>
                      </TableCell>
                      <TableCell>{getBankStatusBadge(alias.status)}</TableCell>
                      <TableCell className="text-muted-foreground">
                        {new Date(alias.createdAt).toLocaleDateString()}
                      </TableCell>
                      <TableCell className="text-right">
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" className="h-8 w-8 p-0">
                              <span className="sr-only">Open menu</span>
                              <MoreHorizontal className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem
                              onClick={() => handleViewAccount(alias)}
                            >
                              <ExternalLink className="mr-2 h-4 w-4" />
                              View Account
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              onClick={() =>
                                customer && handleViewCustomer(customer.id)
                              }
                            >
                              <Users className="mr-2 h-4 w-4" />
                              View Customer
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              onClick={() => handleEditAlias(alias)}
                            >
                              <Edit3 className="mr-2 h-4 w-4" />
                              Edit Alias
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              onClick={() => handleDeleteAlias(alias)}
                              className="text-destructive"
                            >
                              <Trash2 className="mr-2 h-4 w-4" />
                              Delete
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </React.Fragment>
  )
}

export default AliasesPage
