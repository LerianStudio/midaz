'use client'

import React, { useState } from 'react'
import { useParams } from 'next/navigation'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
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
  MoreHorizontal,
  Edit3,
  Trash2,
  Building2,
  Users,
  ExternalLink,
  Banknote
} from 'lucide-react'
import {
  generateMockCustomers,
  generateMockAliases
} from '@/components/crm/customers/customer-mock-data'
import { Customer, Alias } from '@/components/crm/customers/customer-types'

export default function CustomerAliasesPage() {
  const params = useParams()
  const customerId = params.id as string
  const [searchTerm, setSearchTerm] = useState('')

  // Get customer and aliases from mock data
  const customers = generateMockCustomers(50)
  const customer = customers.find((c) => c.id === customerId)
  const allAliases = generateMockAliases(100)
  const customerAliases = allAliases.filter(
    (alias) => alias.holderId === customerId
  )

  // Filter aliases based on search
  const filteredAliases = customerAliases.filter(
    (alias) =>
      alias.id.toLowerCase().includes(searchTerm.toLowerCase()) ||
      alias.bankingDetails.account
        .toLowerCase()
        .includes(searchTerm.toLowerCase()) ||
      alias.bankingDetails.bankId
        .toLowerCase()
        .includes(searchTerm.toLowerCase())
  )

  if (!customer) {
    return (
      <div className="flex min-h-[400px] flex-col items-center justify-center space-y-4">
        <div className="space-y-2 text-center">
          <h2 className="text-xl font-semibold">Customer Not Found</h2>
          <p className="text-muted-foreground">
            The customer you're looking for doesn't exist or has been removed.
          </p>
        </div>
        <Button onClick={() => window.history.back()}>Go Back</Button>
      </div>
    )
  }

  const handleEditAlias = (alias: Alias) => {
    console.log('Edit alias:', alias)
    // TODO: Open edit alias dialog
  }

  const handleDeleteAlias = (alias: Alias) => {
    console.log('Delete alias:', alias)
    // TODO: Show confirmation dialog and delete
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

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-4">
          <div className="flex items-center space-x-2">
            {customer.type === 'NATURAL_PERSON' ? (
              <Users className="h-6 w-6 text-blue-600" />
            ) : (
              <Building2 className="h-6 w-6 text-purple-600" />
            )}
            <div>
              <h1 className="text-2xl font-bold">{customer.name}</h1>
              <p className="text-muted-foreground">
                Banking Aliases & Account Links
              </p>
            </div>
          </div>
        </div>
        <Button>
          <Plus className="mr-2 h-4 w-4" />
          Create Alias
        </Button>
      </div>

      {/* Search and Stats */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-4">
        <Card className="lg:col-span-3">
          <CardContent className="pt-6">
            <div className="relative">
              <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search aliases by ID, account, or bank..."
                value={searchTerm}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setSearchTerm(e.target.value)
                }
                className="pl-8"
              />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="pt-6">
            <div className="text-center">
              <div className="text-2xl font-bold">{customerAliases.length}</div>
              <p className="text-xs text-muted-foreground">Total Aliases</p>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Aliases Table */}
      <Card>
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
                  : 'Create the first alias for this customer.'}
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
                {filteredAliases.map((alias) => (
                  <TableRow key={alias.id}>
                    <TableCell className="font-medium">
                      <div className="flex items-center space-x-2">
                        <Banknote className="h-4 w-4 text-muted-foreground" />
                        <span className="font-mono text-sm">
                          {alias.id.slice(-8)}
                        </span>
                      </div>
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
                        {alias.bankingDetails.iban && (
                          <div className="font-mono text-xs text-muted-foreground">
                            IBAN: {alias.bankingDetails.iban}
                          </div>
                        )}
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
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {/* Customer Summary */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center space-x-2">
              <CreditCard className="h-5 w-5 text-blue-600" />
              <div>
                <div className="text-2xl font-bold">
                  {
                    customerAliases.filter(
                      (a) => a.bankingDetails.type === 'CHECKING'
                    ).length
                  }
                </div>
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
              <Banknote className="h-5 w-5 text-green-600" />
              <div>
                <div className="text-2xl font-bold">
                  {
                    customerAliases.filter(
                      (a) => a.bankingDetails.type === 'SAVINGS'
                    ).length
                  }
                </div>
                <p className="text-xs text-muted-foreground">
                  Savings Accounts
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center space-x-2">
              <Building2 className="h-5 w-5 text-purple-600" />
              <div>
                <div className="text-2xl font-bold">
                  {customerAliases.filter((a) => a.status === 'active').length}
                </div>
                <p className="text-xs text-muted-foreground">Active Aliases</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
