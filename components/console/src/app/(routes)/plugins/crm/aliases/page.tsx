'use client'

import React, { useState, useEffect, useTransition } from 'react'
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
  Banknote,
  Loader2
} from 'lucide-react'
import { getAllAliases, getHolders, deleteAlias } from '@/app/actions/crm'
import { AliasEntity } from '@/core/domain/entities/alias-entity'
import { HolderEntity } from '@/core/domain/entities/holder-entity'
import { useToast } from '@/hooks/use-toast'

const AliasesPage = () => {
  const intl = useIntl()
  const router = useRouter()
  const { currentOrganization } = useOrganization()
  const { toast } = useToast()
  const [searchTerm, setSearchTerm] = useState('')
  const [aliases, setAliases] = useState<AliasEntity[]>([])
  const [holders, setHolders] = useState<HolderEntity[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isPending, startTransition] = useTransition()
  const [totalAliases, setTotalAliases] = useState(0)

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

  // Fetch data on mount
  useEffect(() => {
    const fetchData = async () => {
      setIsLoading(true)
      try {
        // Fetch aliases and holders in parallel
        const [aliasesResult, holdersResult] = await Promise.all([
          getAllAliases({ organizationId: currentOrganization.id, limit: 100 }),
          getHolders({ organizationId: currentOrganization.id, limit: 100 })
        ])

        if (aliasesResult.success && aliasesResult.data) {
          setAliases(aliasesResult.data.aliases)
          setTotalAliases(aliasesResult.data.total)
        } else if (
          aliasesResult.error &&
          !aliasesResult.error.includes('404')
        ) {
          // Only show error for non-404 errors (404 means no data found, which is OK)
          toast({
            title: 'Error fetching aliases',
            description: aliasesResult.error,
            variant: 'destructive'
          })
        }

        if (holdersResult.success && holdersResult.data) {
          setHolders(holdersResult.data.holders)
        }
      } catch (error) {
        console.error('Error fetching data:', error)
      } finally {
        setIsLoading(false)
      }
    }

    fetchData()
  }, [currentOrganization.id, toast])

  // Create holder lookup map
  const holderMap = holders.reduce(
    (acc, holder) => {
      acc[holder.id] = holder
      return acc
    },
    {} as Record<string, HolderEntity>
  )

  // Filter aliases based on search
  const filteredAliases = aliases.filter((alias) => {
    const holder = holderMap[alias.holderId]
    return (
      alias.id.toLowerCase().includes(searchTerm.toLowerCase()) ||
      alias.document.toLowerCase().includes(searchTerm.toLowerCase()) ||
      (alias.bankingDetails?.account || '')
        .toLowerCase()
        .includes(searchTerm.toLowerCase()) ||
      (alias.bankingDetails?.bankId || '')
        .toLowerCase()
        .includes(searchTerm.toLowerCase()) ||
      holder?.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      holder?.document.includes(searchTerm)
    )
  })

  const handleEditAlias = (alias: AliasEntity) => {
    console.log('Edit alias:', alias)
    // TODO: Open edit alias dialog
  }

  const handleDeleteAlias = async (alias: AliasEntity) => {
    // Find the holder for this alias
    const holder = holders.find((h) => h.id === alias.holderId)
    if (!holder) {
      toast({
        title: 'Error',
        description: 'Could not find holder for this alias',
        variant: 'destructive'
      })
      return
    }

    if (confirm('Are you sure you want to delete this alias?')) {
      startTransition(async () => {
        const result = await deleteAlias(holder.id, alias.id)
        if (result.success) {
          setAliases((prev) => prev.filter((a) => a.id !== alias.id))
          toast({
            title: 'Success',
            description: 'Alias deleted successfully'
          })
        } else {
          toast({
            title: 'Error',
            description: result.error || 'Failed to delete alias',
            variant: 'destructive'
          })
        }
      })
    }
  }

  const handleViewHolder = (holderId: string) => {
    router.push(`/plugins/crm/holders/${holderId}`)
  }

  const handleViewAccount = (alias: AliasEntity) => {
    console.log('View account:', alias.accountId)
    // TODO: Navigate to account detail page
  }

  const getAliasTypeBadge = (type: string) => {
    switch (type.toUpperCase()) {
      case 'NATURAL_PERSON':
        return (
          <Badge variant="outline" className="bg-blue-100 text-blue-800">
            Natural Person
          </Badge>
        )
      case 'LEGAL_PERSON':
        return (
          <Badge variant="outline" className="bg-green-100 text-green-800">
            Legal Person
          </Badge>
        )
      case 'BANK_ACCOUNT':
        return (
          <Badge variant="outline" className="bg-purple-100 text-purple-800">
            Bank Account
          </Badge>
        )
      case 'PIX':
        return (
          <Badge variant="outline" className="bg-orange-100 text-orange-800">
            PIX
          </Badge>
        )
      default:
        return <Badge variant="outline">{type}</Badge>
    }
  }

  const getHolderTypeIcon = (type: string) => {
    return type === 'NATURAL_PERSON' ? (
      <Users className="h-4 w-4 text-blue-600" />
    ) : (
      <Building2 className="h-4 w-4 text-purple-600" />
    )
  }

  // Calculate metrics
  const metrics = {
    total: totalAliases,
    naturalPerson: aliases.filter((a) => a.type === 'NATURAL_PERSON').length,
    legalPerson: aliases.filter((a) => a.type === 'LEGAL_PERSON').length,
    other: aliases.filter((a) => !['NATURAL_PERSON', 'LEGAL_PERSON'].includes(a.type))
      .length
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
                <div className="text-2xl font-bold">{metrics.naturalPerson}</div>
                <p className="text-xs text-muted-foreground">Natural Persons</p>
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
                <div className="text-2xl font-bold">{metrics.legalPerson}</div>
                <p className="text-xs text-muted-foreground">Legal Persons</p>
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
                <div className="text-2xl font-bold">{metrics.other}</div>
                <p className="text-xs text-muted-foreground">Other Aliases</p>
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
          {isLoading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          ) : filteredAliases.length === 0 ? (
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
                  <TableHead>Holder</TableHead>
                  <TableHead>Document</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Banking Details</TableHead>
                  <TableHead>Ledger</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredAliases.map((alias) => {
                  // Find holder by matching holderId
                  const holder = holders.find((h) => h.id === alias.holderId)
                  return (
                    <TableRow key={alias.id}>
                      <TableCell>
                        {holder ? (
                          <div className="flex items-center space-x-3">
                            <div className="flex-shrink-0">
                              {getHolderTypeIcon(holder.type)}
                            </div>
                            <div>
                              <button
                                onClick={() => handleViewHolder(holder.id)}
                                className="text-left font-medium hover:underline"
                              >
                                {holder.name}
                              </button>
                              <div className="text-sm text-muted-foreground">
                                {holder.document}
                              </div>
                            </div>
                          </div>
                        ) : (
                          <span className="text-muted-foreground">Unknown</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="font-medium">{alias.document}</div>
                        <div className="font-mono text-xs text-muted-foreground">
                          {alias.id.slice(-8)}
                        </div>
                      </TableCell>
                      <TableCell>{getAliasTypeBadge(alias.type)}</TableCell>
                      <TableCell>
                        {alias.bankingDetails ? (
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
                            {alias.bankingDetails.type && (
                              <div className="text-xs text-muted-foreground">
                                Type: {alias.bankingDetails.type} ({alias.bankingDetails.countryCode})
                              </div>
                            )}
                          </div>
                        ) : (
                          <span className="text-muted-foreground">-</span>
                        )}
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
                      <TableCell className="text-muted-foreground">
                        {new Date(alias.createdAt).toLocaleDateString()}
                      </TableCell>
                      <TableCell className="text-right">
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              className="h-8 w-8 p-0"
                              disabled={isPending}
                            >
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
                            {holder && (
                              <DropdownMenuItem
                                onClick={() => handleViewHolder(holder.id)}
                              >
                                <Users className="mr-2 h-4 w-4" />
                                View Holder
                              </DropdownMenuItem>
                            )}
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
