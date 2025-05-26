'use client'

import { useState } from 'react'
import { useRouter, useParams } from 'next/navigation'
import {
  ArrowLeft,
  Save,
  Eye,
  Download,
  Upload,
  Undo,
  Redo
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { PageHeader } from '@/components/page-header'
import { useToast } from '@/hooks/use-toast'

import { TransactionRouteDesigner } from '@/components/accounting/transaction-routes/transaction-route-designer'
import {
  mockTransactionRoutes,
  getTransactionRouteById,
  type TransactionRoute
} from '@/components/accounting/mock/transaction-route-mock-data'

const statusColors = {
  active: 'bg-green-100 text-green-800 border-green-200',
  draft: 'bg-yellow-100 text-yellow-800 border-yellow-200',
  deprecated: 'bg-red-100 text-red-800 border-red-200'
}

export default function TransactionRouteDesignerPage() {
  const router = useRouter()
  const params = useParams()
  const { toast } = useToast()
  const routeId = params.id as string

  const [originalRoute] = useState<TransactionRoute | null>(
    getTransactionRouteById(routeId) || mockTransactionRoutes[0]
  )
  const [route, setRoute] = useState<TransactionRoute | null>(originalRoute)
  const [hasChanges, setHasChanges] = useState(false)
  const [isSaving, setIsSaving] = useState(false)

  if (!route || !originalRoute) {
    return (
      <div className="space-y-6">
        <PageHeader.Root>
          <div className="flex items-center space-x-4">
            <Button variant="ghost" size="sm" onClick={() => router.back()}>
              <ArrowLeft className="h-4 w-4" />
            </Button>
            <PageHeader.InfoTitle>Route Not Found</PageHeader.InfoTitle>
          </div>
        </PageHeader.Root>
        <Card>
          <CardContent className="p-8 text-center">
            <p>The requested transaction route could not be found.</p>
          </CardContent>
        </Card>
      </div>
    )
  }

  const handleRouteChange = (updatedRoute: TransactionRoute) => {
    setRoute(updatedRoute)
    setHasChanges(
      JSON.stringify(updatedRoute) !== JSON.stringify(originalRoute)
    )
  }

  const handleSave = async () => {
    setIsSaving(true)
    try {
      // In a real implementation, this would call an API to save the route
      console.log('Saving route:', route)

      // Simulate API call
      await new Promise((resolve) => setTimeout(resolve, 1000))

      toast({
        title: 'Route Saved',
        description: 'Transaction route has been successfully updated.'
      })

      setHasChanges(false)
    } catch (error) {
      toast({
        title: 'Save Failed',
        description: 'Failed to save the transaction route. Please try again.',
        variant: 'destructive'
      })
    } finally {
      setIsSaving(false)
    }
  }

  const handlePreview = () => {
    router.push(`/plugins/accounting/transaction-routes/${routeId}`)
  }

  const handleExport = () => {
    const dataStr = JSON.stringify(route, null, 2)
    const dataBlob = new Blob([dataStr], { type: 'application/json' })
    const url = URL.createObjectURL(dataBlob)
    const link = document.createElement('a')
    link.href = url
    link.download = `transaction-route-${route.name.toLowerCase().replace(/\s+/g, '-')}.json`
    link.click()
    URL.revokeObjectURL(url)
  }

  const handleImport = () => {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.json'
    input.onchange = (e) => {
      const file = (e.target as HTMLInputElement).files?.[0]
      if (file) {
        const reader = new FileReader()
        reader.onload = (e) => {
          try {
            const importedRoute = JSON.parse(e.target?.result as string)
            setRoute(importedRoute)
            setHasChanges(true)
            toast({
              title: 'Route Imported',
              description:
                'Transaction route configuration has been imported successfully.'
            })
          } catch (error) {
            toast({
              title: 'Import Failed',
              description:
                'Failed to parse the imported file. Please check the file format.',
              variant: 'destructive'
            })
          }
        }
        reader.readAsText(file)
      }
    }
    input.click()
  }

  const handleReset = () => {
    if (hasChanges) {
      if (
        confirm(
          'Are you sure you want to reset all changes? This action cannot be undone.'
        )
      ) {
        setRoute(originalRoute)
        setHasChanges(false)
        toast({
          title: 'Changes Reset',
          description:
            'All changes have been reset to the original configuration.'
        })
      }
    }
  }

  const validationIssues = []

  // Basic validation
  if (route.operationRoutes.length === 0) {
    validationIssues.push('Route must have at least one operation')
  }

  if (route.operationRoutes.some((op) => !op.description.trim())) {
    validationIssues.push('All operations must have descriptions')
  }

  const orderNumbers = route.operationRoutes.map((op) => op.order)
  if (new Set(orderNumbers).size !== orderNumbers.length) {
    validationIssues.push('Operation orders must be unique')
  }

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <div className="flex w-full items-center justify-between">
          <div className="flex items-center space-x-4">
            <Button variant="ghost" size="sm" onClick={() => router.back()}>
              <ArrowLeft className="h-4 w-4" />
            </Button>
            <div>
              <div className="flex items-center space-x-3">
                <PageHeader.InfoTitle>
                  Edit Route: {route.name}
                </PageHeader.InfoTitle>
                <Badge className={statusColors[route.status]}>
                  {route.status}
                </Badge>
                {hasChanges && (
                  <Badge variant="outline" className="text-xs">
                    Unsaved Changes
                  </Badge>
                )}
              </div>
              <PageHeader.InfoTooltip>
                Visual designer for configuring transaction route operations.
              </PageHeader.InfoTooltip>
            </div>
          </div>

          <div className="flex items-center space-x-2">
            <Button variant="outline" onClick={handleImport}>
              <Upload className="mr-2 h-4 w-4" />
              Import
            </Button>
            <Button variant="outline" onClick={handleExport}>
              <Download className="mr-2 h-4 w-4" />
              Export
            </Button>
            <Button
              variant="outline"
              onClick={handleReset}
              disabled={!hasChanges}
            >
              <Undo className="mr-2 h-4 w-4" />
              Reset
            </Button>
            <Button variant="outline" onClick={handlePreview}>
              <Eye className="mr-2 h-4 w-4" />
              Preview
            </Button>
            <Button
              onClick={handleSave}
              disabled={!hasChanges || validationIssues.length > 0 || isSaving}
              className="gap-2"
            >
              <Save className="h-4 w-4" />
              {isSaving ? 'Saving...' : 'Save Changes'}
            </Button>
          </div>
        </div>
      </PageHeader.Root>

      {/* Validation Alerts */}
      {validationIssues.length > 0 && (
        <Alert className="border-red-200 bg-red-50">
          <AlertDescription>
            <div>
              <strong>Validation Issues:</strong>
              <ul className="mt-1 list-inside list-disc">
                {validationIssues.map((issue, index) => (
                  <li key={index} className="text-sm">
                    {issue}
                  </li>
                ))}
              </ul>
            </div>
          </AlertDescription>
        </Alert>
      )}

      {/* Route Stats */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-5">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Operations</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {route.operationRoutes.length}
            </div>
            <p className="text-xs text-muted-foreground">Total operations</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Debits</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-red-600">
              {
                route.operationRoutes.filter(
                  (op) => op.operationType === 'debit'
                ).length
              }
            </div>
            <p className="text-xs text-muted-foreground">Debit operations</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Credits</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-600">
              {
                route.operationRoutes.filter(
                  (op) => op.operationType === 'credit'
                ).length
              }
            </div>
            <p className="text-xs text-muted-foreground">Credit operations</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Version</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{route.version}</div>
            <p className="text-xs text-muted-foreground">Current version</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Status</CardTitle>
          </CardHeader>
          <CardContent>
            <Badge className={statusColors[route.status]}>{route.status}</Badge>
            <p className="mt-1 text-xs text-muted-foreground">Route status</p>
          </CardContent>
        </Card>
      </div>

      <Tabs defaultValue="designer" className="w-full">
        <TabsList>
          <TabsTrigger value="designer">Visual Designer</TabsTrigger>
          <TabsTrigger value="configuration">Configuration</TabsTrigger>
          <TabsTrigger value="validation">Validation</TabsTrigger>
        </TabsList>

        <TabsContent value="designer" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Route Designer</CardTitle>
              <CardDescription>
                Drag and drop to configure your transaction route operations.
                Add operations, configure account mappings, and set up the flow.
              </CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <TransactionRouteDesigner
                route={route}
                onChange={handleRouteChange}
                mode="edit"
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="configuration" className="space-y-6">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Route Information</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Name
                  </label>
                  <p className="text-sm">{route.name}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Description
                  </label>
                  <p className="text-sm">{route.description}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Type
                  </label>
                  <div className="mt-1">
                    <Badge>{route.templateType}</Badge>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Metadata</CardTitle>
              </CardHeader>
              <CardContent>
                {Object.keys(route.metadata).length > 0 ? (
                  <pre className="overflow-x-auto rounded bg-muted p-3 text-xs">
                    {JSON.stringify(route.metadata, null, 2)}
                  </pre>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    No metadata configured
                  </p>
                )}
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Tags</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex flex-wrap gap-2">
                {route.tags.map((tag) => (
                  <Badge key={tag} variant="outline">
                    {tag}
                  </Badge>
                ))}
                {route.tags.length === 0 && (
                  <p className="text-sm text-muted-foreground">
                    No tags assigned
                  </p>
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="validation" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Validation Results</CardTitle>
              <CardDescription>
                Check for issues and compliance with transaction route rules.
              </CardDescription>
            </CardHeader>
            <CardContent>
              {validationIssues.length === 0 ? (
                <div className="py-8 text-center">
                  <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-green-100">
                    <div className="h-8 w-8 rounded-full bg-green-500"></div>
                  </div>
                  <h3 className="text-lg font-medium text-green-800">
                    All Good!
                  </h3>
                  <p className="mt-1 text-sm text-green-600">
                    Your transaction route passes all validation checks.
                  </p>
                </div>
              ) : (
                <div className="space-y-4">
                  {validationIssues.map((issue, index) => (
                    <Alert key={index} className="border-red-200 bg-red-50">
                      <AlertDescription className="text-red-800">
                        {issue}
                      </AlertDescription>
                    </Alert>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Route Statistics</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-2 gap-4 text-sm">
                <div>
                  <label className="font-medium text-muted-foreground">
                    Total Operations
                  </label>
                  <p>{route.operationRoutes.length}</p>
                </div>
                <div>
                  <label className="font-medium text-muted-foreground">
                    Debit/Credit Balance
                  </label>
                  <p>
                    {
                      route.operationRoutes.filter(
                        (op) => op.operationType === 'debit'
                      ).length
                    }
                    /
                    {
                      route.operationRoutes.filter(
                        (op) => op.operationType === 'credit'
                      ).length
                    }
                  </p>
                </div>
                <div>
                  <label className="font-medium text-muted-foreground">
                    Conditional Operations
                  </label>
                  <p>
                    {
                      route.operationRoutes.filter(
                        (op) => op.conditions && op.conditions.length > 0
                      ).length
                    }
                  </p>
                </div>
                <div>
                  <label className="font-medium text-muted-foreground">
                    Unique Account Types
                  </label>
                  <p>
                    {
                      new Set([
                        ...route.operationRoutes.map(
                          (op) => op.sourceAccountTypeId
                        ),
                        ...route.operationRoutes.map(
                          (op) => op.destinationAccountTypeId
                        )
                      ]).size
                    }
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
