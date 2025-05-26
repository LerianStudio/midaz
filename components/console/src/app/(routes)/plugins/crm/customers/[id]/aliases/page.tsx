'use client'

import React, { useState, useEffect, useTransition } from 'react'
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
  Banknote,
  Loader2
} from 'lucide-react'
import {
  getHolderById,
  getAliasesByHolderId,
  deleteAlias
} from '@/app/actions/crm'
import { HolderEntity } from '@/core/domain/entities/holder-entity'
import { AliasEntity } from '@/core/domain/entities/alias-entity'
import { useToast } from '@/hooks/use-toast'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'

export default function CustomerAliasesPage() {
  const params = useParams()
  const holderId = params.id as string
  const { toast } = useToast()
  const { currentOrganization } = useOrganization()
  const [searchTerm, setSearchTerm] = useState('')
  const [holder, setHolder] = useState<HolderEntity | null>(null)
  const [aliases, setAliases] = useState<AliasEntity[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isPending, startTransition] = useTransition()
  const [totalAliases, setTotalAliases] = useState(0)

  // Fetch holder and aliases on mount
  useEffect(() => {
    const fetchData = async () => {
      setIsLoading(true)
      try {
        // Fetch holder details
        const holderResult = await getHolderById(holderId)
        if (holderResult.success && holderResult.data) {
          setHolder(holderResult.data)

          // Fetch aliases for this holder
          const aliasesResult = await getAliasesByHolderId({
            holderId,
            organizationId: currentOrganization.id,
            limit: 100
          })

          if (aliasesResult.success && aliasesResult.data) {
            setAliases(aliasesResult.data.aliases)
            setTotalAliases(aliasesResult.data.total)
          } else if (aliasesResult.error) {
            toast({
              title: 'Error fetching aliases',
              description: aliasesResult.error,
              variant: 'destructive'
            })
          }
        } else if (holderResult.error) {
          toast({
            title: 'Error fetching holder',
            description: holderResult.error,
            variant: 'destructive'
          })
        }
      } catch (error) {
        toast({
          title: 'Error',
          description: 'Failed to load data',
          variant: 'destructive'
        })
      } finally {
        setIsLoading(false)
      }
    }

    fetchData()
  }, [holderId, currentOrganization.id, toast])

  // Filter aliases based on search
  const filteredAliases = aliases.filter(
    (alias) =>
      alias.id.toLowerCase().includes(searchTerm.toLowerCase()) ||
      alias.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      (alias.bankAccount?.number || '')
        .toLowerCase()
        .includes(searchTerm.toLowerCase()) ||
      (alias.bankAccount?.bankCode || '')
        .toLowerCase()
        .includes(searchTerm.toLowerCase())
  )

  if (isLoading) {
    return (
      <div className="flex min-h-[400px] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!holder) {
    return (
      <div className="flex min-h-[400px] flex-col items-center justify-center space-y-4">
        <div className="space-y-2 text-center">
          <h2 className="text-xl font-semibold">Holder Not Found</h2>
          <p className="text-muted-foreground">
            The holder you&apos;re looking for doesn&apos;t exist or has been
            removed.
          </p>
        </div>
        <Button onClick={() => window.history.back()}>Go Back</Button>
      </div>
    )
  }

  const handleEditAlias = (alias: AliasEntity) => {
    console.log('Edit alias:', alias)
    // TODO: Open edit alias dialog
  }

  const handleDeleteAlias = async (alias: AliasEntity) => {
    if (confirm('Are you sure you want to delete this alias?')) {
      startTransition(async () => {
        const result = await deleteAlias(holderId, alias.id)
        if (result.success) {
          setAliases((prev) => prev.filter((a) => a.id !== alias.id))
          setTotalAliases((prev) => prev - 1)
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

  const handleViewAccount = (alias: AliasEntity) => {
    console.log('View account:', alias.accountId)
    // TODO: Navigate to account detail page
  }

  const getAliasTypeBadge = (type: string) => {
    switch (type.toLowerCase()) {
      case 'bank_account':
        return (
          <Badge variant="outline" className="bg-blue-100 text-blue-800">
            Bank Account
          </Badge>
        )
      case 'pix':
        return (
          <Badge variant="outline" className="bg-green-100 text-green-800">
            PIX
          </Badge>
        )
      case 'email':
        return (
          <Badge variant="outline" className="bg-purple-100 text-purple-800">
            Email
          </Badge>
        )
      case 'phone':
        return (
          <Badge variant="outline" className="bg-orange-100 text-orange-800">
            Phone
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
            {holder.type === 'NATURAL_PERSON' ? (
              <Users className="h-6 w-6 text-blue-600" />
            ) : (
              <Building2 className="h-6 w-6 text-purple-600" />
            )}
            <div>
              <h1 className="text-2xl font-bold">{holder.name}</h1>
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
              <div className="text-2xl font-bold">{totalAliases}</div>
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
                  <TableHead>Alias Name</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Details</TableHead>
                  <TableHead>Ledger</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredAliases.map((alias) => (
                  <TableRow key={alias.id}>
                    <TableCell>
                      <div className="font-medium">{alias.name}</div>
                      <div className="font-mono text-xs text-muted-foreground">
                        {alias.id.slice(-8)}
                      </div>
                    </TableCell>
                    <TableCell>{getAliasTypeBadge(alias.type)}</TableCell>
                    <TableCell>
                      {alias.bankAccount ? (
                        <div className="space-y-1">
                          <div className="flex items-center space-x-2">
                            <span className="font-medium">
                              {alias.bankAccount.bankCode}
                            </span>
                            <span className="text-muted-foreground">•</span>
                            <span className="text-muted-foreground">
                              Branch {alias.bankAccount.branch}
                            </span>
                          </div>
                          <div className="text-sm text-muted-foreground">
                            Account: {alias.bankAccount.number}
                          </div>
                          {alias.bankAccount.type && (
                            <div className="text-xs text-muted-foreground">
                              Type: {alias.bankAccount.type}
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

      {/* Holder Summary */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center space-x-2">
              <CreditCard className="h-5 w-5 text-blue-600" />
              <div>
                <div className="text-2xl font-bold">
                  {aliases.filter((a) => a.type === 'bank_account').length}
                </div>
                <p className="text-xs text-muted-foreground">Bank Accounts</p>
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
                  {aliases.filter((a) => a.type === 'pix').length}
                </div>
                <p className="text-xs text-muted-foreground">PIX Keys</p>
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
                  {
                    aliases.filter(
                      (a) => !['bank_account', 'pix'].includes(a.type)
                    ).length
                  }
                </div>
                <p className="text-xs text-muted-foreground">Other Aliases</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
