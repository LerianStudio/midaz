'use client'

import React from 'react'
import { FeePackage } from '../types/fee-types'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import { Card } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import {
  MoreVertical,
  Edit,
  Trash2,
  Power,
  Copy,
  Eye,
  Users
} from 'lucide-react'
import { PackageStatusBadge } from './package-status-badge'
import { useIntl } from 'react-intl'
import { Skeleton } from '@/components/ui/skeleton'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from '@/components/ui/alert-dialog'

interface PackageDataTableProps {
  packages: FeePackage[]
  isLoading?: boolean
  onEdit?: (pkg: FeePackage) => void
  onDelete?: (id: string) => void
  onToggleStatus?: (id: string) => void
}

export function PackageDataTable({
  packages,
  isLoading = false,
  onEdit,
  onDelete,
  onToggleStatus
}: PackageDataTableProps) {
  const intl = useIntl()
  const [deleteDialogOpen, setDeleteDialogOpen] = React.useState(false)
  const [selectedPackageId, setSelectedPackageId] = React.useState<
    string | null
  >(null)

  const handleDeleteClick = (id: string) => {
    setSelectedPackageId(id)
    setDeleteDialogOpen(true)
  }

  const handleDeleteConfirm = () => {
    if (selectedPackageId && onDelete) {
      onDelete(selectedPackageId)
    }
    setDeleteDialogOpen(false)
    setSelectedPackageId(null)
  }

  const calculateTypesSummary = (pkg: FeePackage) => {
    const types = pkg.types
      .map((t) => t.type)
      .filter((v, i, a) => a.indexOf(v) === i)
    return types.join(', ')
  }

  if (isLoading) {
    return (
      <Card>
        <div className="space-y-4 p-6">
          {[...Array(5)].map((_, i) => (
            <div key={i} className="flex items-center space-x-4">
              <Skeleton className="h-12 w-12 rounded-full" />
              <div className="flex-1 space-y-2">
                <Skeleton className="h-4 w-[250px]" />
                <Skeleton className="h-4 w-[200px]" />
              </div>
            </div>
          ))}
        </div>
      </Card>
    )
  }

  if (packages.length === 0) {
    return (
      <Card>
        <div className="p-12 text-center">
          <p className="text-muted-foreground">
            {intl.formatMessage({
              id: 'fees.packages.emptyState',
              defaultMessage:
                'No fee packages found. Create your first package to get started.'
            })}
          </p>
        </div>
      </Card>
    )
  }

  return (
    <>
      <Card>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Package Name</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Types</TableHead>
              <TableHead>Waived Accounts</TableHead>
              <TableHead>Created</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {packages.map((pkg) => (
              <TableRow key={pkg.id}>
                <TableCell>
                  <div>
                    <p className="font-medium">{pkg.name}</p>
                    {pkg.metadata?.description && (
                      <p className="text-sm text-muted-foreground">
                        {pkg.metadata.description}
                      </p>
                    )}
                  </div>
                </TableCell>
                <TableCell>
                  <PackageStatusBadge active={pkg.active} />
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {pkg.types.map((type, index) => (
                      <Badge
                        key={index}
                        variant="secondary"
                        className="text-xs"
                      >
                        {type.type}
                      </Badge>
                    ))}
                  </div>
                </TableCell>
                <TableCell>
                  {pkg.waivedAccounts.length > 0 ? (
                    <div className="flex items-center gap-1">
                      <Users className="h-4 w-4 text-muted-foreground" />
                      <span className="text-sm">
                        {pkg.waivedAccounts.length}
                      </span>
                    </div>
                  ) : (
                    <span className="text-sm text-muted-foreground">None</span>
                  )}
                </TableCell>
                <TableCell>
                  <span className="text-sm text-muted-foreground">
                    {new Date(pkg.createdAt).toLocaleDateString()}
                  </span>
                </TableCell>
                <TableCell className="text-right">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" className="h-8 w-8 p-0">
                        <span className="sr-only">Open menu</span>
                        <MoreVertical className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => onEdit?.(pkg)}>
                        <Eye className="mr-2 h-4 w-4" />
                        View Details
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => onEdit?.(pkg)}>
                        <Edit className="mr-2 h-4 w-4" />
                        Edit Package
                      </DropdownMenuItem>
                      <DropdownMenuItem>
                        <Copy className="mr-2 h-4 w-4" />
                        Duplicate
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        onClick={() => onToggleStatus?.(pkg.id)}
                      >
                        <Power className="mr-2 h-4 w-4" />
                        {pkg.active ? 'Deactivate' : 'Activate'}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={() => handleDeleteClick(pkg.id)}
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
      </Card>

      <AlertDialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Are you sure?</AlertDialogTitle>
            <AlertDialogDescription>
              This action cannot be undone. This will permanently delete the fee
              package and remove it from all associated transactions.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDeleteConfirm}
              className="bg-destructive text-destructive-foreground"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
