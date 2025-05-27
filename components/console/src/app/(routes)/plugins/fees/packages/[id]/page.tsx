'use client'

import React from 'react'
import { useParams, useRouter } from 'next/navigation'
import { useIntl } from 'react-intl'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  ArrowLeft,
  Edit,
  Trash2,
  Copy,
  Package,
  Calculator,
  Users,
  Activity,
  TrendingUp,
  Settings
} from 'lucide-react'
import { getPackageById } from '@/components/fees/mock/fee-mock-data'
import { PackageStatusBadge } from '@/components/fees/packages/package-status-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { useToast } from '@/hooks/use-toast'
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

export default function PackageDetailsPage() {
  const params = useParams()
  const router = useRouter()
  const intl = useIntl()
  const { toast } = useToast()
  const [isLoading, setIsLoading] = React.useState(true)
  const [deleteDialogOpen, setDeleteDialogOpen] = React.useState(false)

  const packageId = params.id as string
  const pkg = getPackageById(packageId)

  React.useEffect(() => {
    // Simulate loading
    setTimeout(() => setIsLoading(false), 500)
  }, [])

  const handleDelete = async () => {
    // Mock delete - in real app would call API
    toast({
      title: 'Package deleted',
      description: 'The fee package has been deleted successfully.',
      variant: 'success'
    })
    router.push('/plugins/fees/packages')
  }

  const handleDuplicate = () => {
    // In real app would navigate to create page with pre-filled data
    router.push(`/plugins/fees/packages/create?duplicate=${packageId}`)
  }

  if (isLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-64" />
        <div className="grid gap-6">
          <Skeleton className="h-32" />
          <Skeleton className="h-64" />
        </div>
      </div>
    )
  }

  if (!pkg) {
    return (
      <div className="py-12 text-center">
        <p className="text-muted-foreground">Package not found</p>
        <Button
          variant="outline"
          onClick={() => router.push('/plugins/fees/packages')}
          className="mt-4"
        >
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Packages
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <PageHeader.Wrapper>
          <div className="flex items-center gap-4">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => router.push('/plugins/fees/packages')}
            >
              <ArrowLeft className="h-4 w-4" />
            </Button>
            <PageHeader.InfoTitle
              title={pkg.name}
              subtitle={pkg.metadata?.description || 'Fee calculation package'}
            />
            <PackageStatusBadge active={pkg.active} />
          </div>
          <PageHeader.ActionButtons>
            <Button variant="outline" size="sm" onClick={handleDuplicate}>
              <Copy className="mr-2 h-4 w-4" />
              Duplicate
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setDeleteDialogOpen(true)}
              className="text-destructive hover:text-destructive"
            >
              <Trash2 className="mr-2 h-4 w-4" />
              Delete
            </Button>
            <Button
              size="sm"
              onClick={() =>
                router.push(`/plugins/fees/packages/${packageId}/edit`)
              }
            >
              <Edit className="mr-2 h-4 w-4" />
              Edit Package
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>
      </PageHeader.Root>

      <Tabs defaultValue="overview" className="space-y-4">
        <TabsList>
          <TabsTrigger value="overview" className="flex items-center gap-2">
            <Package className="h-4 w-4" />
            Overview
          </TabsTrigger>
          <TabsTrigger value="rules" className="flex items-center gap-2">
            <Calculator className="h-4 w-4" />
            Fee Rules
          </TabsTrigger>
          <TabsTrigger value="waivers" className="flex items-center gap-2">
            <Users className="h-4 w-4" />
            Waivers
          </TabsTrigger>
          <TabsTrigger value="analytics" className="flex items-center gap-2">
            <TrendingUp className="h-4 w-4" />
            Analytics
          </TabsTrigger>
          <TabsTrigger value="settings" className="flex items-center gap-2">
            <Settings className="h-4 w-4" />
            Settings
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview">
          <div className="grid gap-6">
            {/* Package Info */}
            <Card>
              <CardHeader>
                <CardTitle>Package Information</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <p className="text-sm text-muted-foreground">Package ID</p>
                    <p className="font-mono text-sm">{pkg.id}</p>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Ledger ID</p>
                    <p className="font-mono text-sm">{pkg.ledgerId}</p>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Created</p>
                    <p className="text-sm">
                      {new Date(pkg.createdAt).toLocaleDateString()}
                    </p>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">
                      Last Updated
                    </p>
                    <p className="text-sm">
                      {new Date(pkg.updatedAt).toLocaleDateString()}
                    </p>
                  </div>
                </div>

                {pkg.metadata && Object.keys(pkg.metadata).length > 0 && (
                  <div>
                    <p className="mb-2 text-sm text-muted-foreground">
                      Metadata
                    </p>
                    <div className="space-y-1 rounded-lg bg-muted/50 p-3">
                      {Object.entries(pkg.metadata).map(([key, value]) => (
                        <div key={key} className="flex justify-between text-sm">
                          <span className="text-muted-foreground">{key}:</span>
                          <span>{String(value)}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>

            {/* Quick Stats */}
            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
              <Card>
                <CardContent className="p-6">
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="text-sm text-muted-foreground">Fee Rules</p>
                      <p className="text-2xl font-bold">{pkg.types.length}</p>
                    </div>
                    <Calculator className="h-8 w-8 text-muted-foreground/20" />
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardContent className="p-6">
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="text-sm text-muted-foreground">
                        Waived Accounts
                      </p>
                      <p className="text-2xl font-bold">
                        {pkg.waivedAccounts.length}
                      </p>
                    </div>
                    <Users className="h-8 w-8 text-muted-foreground/20" />
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardContent className="p-6">
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="text-sm text-muted-foreground">Status</p>
                      <p className="text-lg font-semibold">
                        {pkg.active ? 'Active' : 'Inactive'}
                      </p>
                    </div>
                    <Activity className="h-8 w-8 text-muted-foreground/20" />
                  </div>
                </CardContent>
              </Card>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="rules">
          <Card>
            <CardHeader>
              <CardTitle>Fee Calculation Rules</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {pkg.types.map((rule, index) => (
                <div key={index} className="space-y-3 rounded-lg border p-4">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <Badge>Priority {rule.priority}</Badge>
                      <span className="text-lg font-medium">{rule.type}</span>
                    </div>
                  </div>

                  {rule.transactionType && (
                    <div className="space-y-1 rounded bg-muted/50 p-3 text-sm">
                      <p className="font-medium">Transaction Criteria:</p>
                      {rule.transactionType.minValue && (
                        <p>Min Value: ${rule.transactionType.minValue}</p>
                      )}
                      {rule.transactionType.maxValue && (
                        <p>Max Value: ${rule.transactionType.maxValue}</p>
                      )}
                      {rule.transactionType.currency && (
                        <p>Currency: {rule.transactionType.currency}</p>
                      )}
                    </div>
                  )}

                  <div className="space-y-2">
                    <p className="text-sm font-medium">Calculation Details:</p>
                    {rule.calculationType.map((calc, calcIndex) => (
                      <div
                        key={calcIndex}
                        className="space-y-1 rounded bg-muted/30 p-2 text-sm"
                      >
                        {calc.value && <p>Fixed Amount: ${calc.value}</p>}
                        {calc.percentage && (
                          <p>Percentage: {calc.percentage}%</p>
                        )}
                        {calc.refAmount && <p>Reference: {calc.refAmount}</p>}
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="waivers">
          <Card>
            <CardHeader>
              <CardTitle>Waived Accounts</CardTitle>
            </CardHeader>
            <CardContent>
              {pkg.waivedAccounts.length === 0 ? (
                <p className="py-8 text-center text-muted-foreground">
                  No accounts are currently waived from this fee package.
                </p>
              ) : (
                <div className="space-y-2">
                  {pkg.waivedAccounts.map((account) => (
                    <div
                      key={account}
                      className="flex items-center justify-between rounded-lg border p-3"
                    >
                      <span className="font-mono text-sm">{account}</span>
                      <Badge variant="secondary">Waived</Badge>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="analytics">
          <Card>
            <CardHeader>
              <CardTitle>Package Analytics</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="py-12 text-center text-muted-foreground">
                <TrendingUp className="mx-auto mb-4 h-12 w-12 opacity-50" />
                <p>
                  Analytics data will be available once the package processes
                  transactions.
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="settings">
          <Card>
            <CardHeader>
              <CardTitle>Package Settings</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between rounded-lg border p-4">
                <div>
                  <p className="font-medium">Package Status</p>
                  <p className="text-sm text-muted-foreground">
                    {pkg.active
                      ? 'Package is currently active and processing fees'
                      : 'Package is inactive'}
                  </p>
                </div>
                <Button variant={pkg.active ? 'outline' : 'default'}>
                  {pkg.active ? 'Deactivate' : 'Activate'}
                </Button>
              </div>

              <div className="border-t pt-4">
                <h4 className="mb-2 font-medium text-destructive">
                  Danger Zone
                </h4>
                <Button
                  variant="outline"
                  className="text-destructive hover:text-destructive"
                  onClick={() => setDeleteDialogOpen(true)}
                >
                  Delete Package
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <AlertDialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Are you absolutely sure?</AlertDialogTitle>
            <AlertDialogDescription>
              This action cannot be undone. This will permanently delete the fee
              package &quot;{pkg.name}&quot; and remove it from all associated
              transactions.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete Package
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
