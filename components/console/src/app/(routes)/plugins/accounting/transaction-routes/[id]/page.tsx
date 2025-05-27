'use client'

import { useState } from 'react'
import { useRouter, useParams } from 'next/navigation'
import {
  ArrowLeft,
  Edit,
  Copy,
  Eye,
  Settings,
  Trash2,
  Play,
  Pause
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import { PageHeader } from '@/components/page-header'

import { TransactionRouteDesigner } from '@/components/accounting/transaction-routes/transaction-route-designer'
import {
  mockTransactionRoutes,
  getTransactionRouteById,
  mockAccountTypes,
  type TransactionRoute
} from '@/components/accounting/mock/transaction-route-mock-data'

const statusColors = {
  active: 'bg-green-100 text-green-800 border-green-200',
  draft: 'bg-yellow-100 text-yellow-800 border-yellow-200',
  deprecated: 'bg-red-100 text-red-800 border-red-200'
}

const templateTypeColors = {
  transfer: 'bg-blue-100 text-blue-800 border-blue-200',
  payment: 'bg-purple-100 text-purple-800 border-purple-200',
  adjustment: 'bg-orange-100 text-orange-800 border-orange-200',
  fee: 'bg-cyan-100 text-cyan-800 border-cyan-200',
  refund: 'bg-pink-100 text-pink-800 border-pink-200',
  custom: 'bg-gray-100 text-gray-800 border-gray-200'
}

export default function TransactionRouteDetailsPage() {
  const router = useRouter()
  const params = useParams()
  const routeId = params.id as string

  const [route, setRoute] = useState<TransactionRoute | null>(
    getTransactionRouteById(routeId) || mockTransactionRoutes[0]
  )

  if (!route) {
    return (
      <div className="space-y-6">
        <PageHeader.Root>
          <div className="flex items-center space-x-4">
            <Button variant="ghost" size="sm" onClick={() => router.back()}>
              <ArrowLeft className="h-4 w-4" />
            </Button>
            <PageHeader.InfoTitle title="Route Not Found" />
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

  const handleEditRoute = () => {
    router.push(`/plugins/accounting/transaction-routes/${routeId}/designer`)
  }

  const handleDuplicateRoute = () => {
    console.log('Duplicating route:', route.name)
    // In a real implementation, this would create a copy
  }

  const handleToggleStatus = () => {
    const newStatus = route.status === 'active' ? 'draft' : 'active'
    setRoute((prev) => (prev ? { ...prev, status: newStatus } : null))
  }

  const handleDeleteRoute = () => {
    console.log('Deleting route:', route.id)
    // In a real implementation, this would show a confirmation dialog
  }

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    })
  }

  const getAccountTypeName = (accountTypeId: string) => {
    return (
      mockAccountTypes.find((at) => at.id === accountTypeId)?.name ||
      'Unknown Account Type'
    )
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
              <PageHeader.InfoTitle title={route.name} />
              <PageHeader.InfoTooltip subtitle={route.description} />
            </div>
          </div>

          <div className="flex items-center space-x-2">
            <Button variant="outline" onClick={handleDuplicateRoute}>
              <Copy className="mr-2 h-4 w-4" />
              Duplicate
            </Button>
            <Button variant="outline" onClick={handleToggleStatus}>
              {route.status === 'active' ? (
                <>
                  <Pause className="mr-2 h-4 w-4" />
                  Deactivate
                </>
              ) : (
                <>
                  <Play className="mr-2 h-4 w-4" />
                  Activate
                </>
              )}
            </Button>
            <Button onClick={handleEditRoute}>
              <Edit className="mr-2 h-4 w-4" />
              Edit Route
            </Button>
          </div>
        </div>
      </PageHeader.Root>

      {/* Route Overview */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Status</CardTitle>
          </CardHeader>
          <CardContent>
            <Badge className={statusColors[route.status]}>{route.status}</Badge>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Type</CardTitle>
          </CardHeader>
          <CardContent>
            <Badge className={templateTypeColors[route.templateType]}>
              {route.templateType}
            </Badge>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Operations</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {route.operationRoutes.length}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Version</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{route.version}</div>
          </CardContent>
        </Card>
      </div>

      <Tabs defaultValue="overview" className="w-full">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="operations">Operations</TabsTrigger>
          <TabsTrigger value="designer">Visual Designer</TabsTrigger>
          <TabsTrigger value="metadata">Metadata</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-6">
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
                    <Badge className={templateTypeColors[route.templateType]}>
                      {route.templateType}
                    </Badge>
                  </div>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Status
                  </label>
                  <div className="mt-1">
                    <Badge className={statusColors[route.status]}>
                      {route.status}
                    </Badge>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Timeline</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Created
                  </label>
                  <p className="text-sm">{formatDate(route.createdAt)}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Last Updated
                  </label>
                  <p className="text-sm">{formatDate(route.updatedAt)}</p>
                </div>
                <div>
                  <label className="text-sm font-medium text-muted-foreground">
                    Version
                  </label>
                  <p className="text-sm">{route.version}</p>
                </div>
              </CardContent>
            </Card>
          </div>

          {route.tags.length > 0 && (
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
                </div>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        <TabsContent value="operations" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Operation Flow</CardTitle>
              <CardDescription>
                Detailed breakdown of all operations in this transaction route.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Order</TableHead>
                    <TableHead>Type</TableHead>
                    <TableHead>Description</TableHead>
                    <TableHead>Source Account</TableHead>
                    <TableHead>Destination Account</TableHead>
                    <TableHead>Amount</TableHead>
                    <TableHead>Conditions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {route.operationRoutes
                    .sort((a, b) => a.order - b.order)
                    .map((operation) => (
                      <TableRow key={operation.id}>
                        <TableCell className="font-medium">
                          {operation.order}
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant={
                              operation.operationType === 'debit'
                                ? 'destructive'
                                : 'secondary'
                            }
                            className="text-xs"
                          >
                            {operation.operationType}
                          </Badge>
                        </TableCell>
                        <TableCell>{operation.description}</TableCell>
                        <TableCell className="text-sm">
                          {getAccountTypeName(operation.sourceAccountTypeId)}
                        </TableCell>
                        <TableCell className="text-sm">
                          {getAccountTypeName(
                            operation.destinationAccountTypeId
                          )}
                        </TableCell>
                        <TableCell>
                          <div className="space-y-1">
                            <div className="font-mono text-xs">
                              {operation.amount.expression}
                            </div>
                            <div className="text-xs text-muted-foreground">
                              {operation.amount.description}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          {operation.conditions &&
                          operation.conditions.length > 0 ? (
                            <Badge variant="outline" className="text-xs">
                              {operation.conditions.length} conditions
                            </Badge>
                          ) : (
                            <span className="text-xs text-muted-foreground">
                              None
                            </span>
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="designer" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Visual Route Designer</CardTitle>
              <CardDescription>
                Interactive view of the transaction route flow and operations.
              </CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <TransactionRouteDesigner
                route={route}
                onChange={setRoute}
                mode="view"
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="metadata" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Route Metadata</CardTitle>
              <CardDescription>
                Configuration and metadata associated with this route.
              </CardDescription>
            </CardHeader>
            <CardContent>
              {Object.keys(route.metadata).length > 0 ? (
                <div className="space-y-4">
                  {Object.entries(route.metadata).map(([key, value]) => (
                    <div key={key}>
                      <label className="text-sm font-medium capitalize text-muted-foreground">
                        {key.replace(/([A-Z])/g, ' $1').toLowerCase()}
                      </label>
                      <div className="mt-1">
                        {typeof value === 'object' ? (
                          <pre className="rounded bg-muted p-2 text-xs">
                            {JSON.stringify(value, null, 2)}
                          </pre>
                        ) : (
                          <p className="text-sm">{String(value)}</p>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">
                  No metadata configured for this route.
                </p>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
