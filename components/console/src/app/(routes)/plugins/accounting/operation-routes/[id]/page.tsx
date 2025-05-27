'use client'

import { useState, useEffect } from 'react'
import { useParams, useRouter } from 'next/navigation'
import {
  ArrowLeft,
  Edit,
  Copy,
  Trash2,
  Building,
  ArrowUpDown,
  Activity,
  CheckCircle,
  AlertTriangle
} from 'lucide-react'
import Link from 'next/link'
import { formatDistanceToNow, format } from 'date-fns'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { useToast } from '@/hooks/use-toast'

import {
  mockOperationRoutes,
  mockAccountTypes,
  type OperationRoute
} from '@/components/accounting/mock/transaction-route-mock-data'

export default function OperationRouteDetailsPage() {
  const params = useParams()
  const router = useRouter()
  const { toast } = useToast()
  const [operationRoute, setOperationRoute] = useState<OperationRoute | null>(
    null
  )
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    // Simulate API call
    const fetchOperationRoute = async () => {
      setIsLoading(true)

      // Simulate network delay
      await new Promise((resolve) => setTimeout(resolve, 500))

      const foundRoute = mockOperationRoutes.find(
        (route) => route.id === params.id
      )
      setOperationRoute(foundRoute || null)
      setIsLoading(false)
    }

    fetchOperationRoute()
  }, [params.id])

  if (isLoading) {
    return (
      <div className="flex h-full flex-col">
        <PageHeader.Root>
          <div className="flex items-center gap-3">
            <Skeleton className="h-8 w-8" />
            <div className="space-y-2">
              <Skeleton className="h-6 w-48" />
              <Skeleton className="h-4 w-64" />
            </div>
          </div>
          <Skeleton className="h-8 w-20" />
        </PageHeader.Root>
        <div className="flex-1 space-y-6 px-6 pb-6">
          <Skeleton className="h-32" />
          <Skeleton className="h-96" />
        </div>
      </div>
    )
  }

  if (!operationRoute) {
    return (
      <div className="flex h-full flex-col items-center justify-center">
        <div className="space-y-4 text-center">
          <h2 className="text-2xl font-semibold">Operation Route Not Found</h2>
          <p className="text-gray-600">
            The operation route you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild>
            <Link href="/plugins/accounting/operation-routes">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Operation Routes
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  const getAccountTypeName = (accountTypeId: string) => {
    return (
      mockAccountTypes.find((at) => at.id === accountTypeId)?.name || 'Unknown'
    )
  }

  const getAccountTypeCode = (accountTypeId: string) => {
    return (
      mockAccountTypes.find((at) => at.id === accountTypeId)?.code || 'UNKNOWN'
    )
  }

  const handleEdit = () => {
    // In a real implementation, this would navigate to edit page
    toast({
      title: 'Edit Operation Route',
      description: 'Edit functionality will be implemented here.'
    })
  }

  const handleDuplicate = () => {
    // In a real implementation, this would create a copy
    toast({
      title: 'Operation Route Duplicated',
      description: `Created a copy of "${operationRoute.description}"`
    })
  }

  const handleDelete = () => {
    // In a real implementation, this would show a confirmation dialog
    if (confirm('Are you sure you want to delete this operation route?')) {
      toast({
        title: 'Operation Route Deleted',
        description: `"${operationRoute.description}" has been deleted.`,
        variant: 'destructive'
      })
      router.push('/plugins/accounting/operation-routes')
    }
  }

  const validationStatus =
    operationRoute.conditions && operationRoute.conditions.length > 0
      ? 'conditional'
      : 'simple'

  return (
    <div className="flex h-full flex-col">
      <PageHeader.Root>
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" asChild>
            <Link href="/plugins/accounting/operation-routes">
              <ArrowLeft className="h-4 w-4" />
            </Link>
          </Button>
          <div>
            <PageHeader.InfoTitle
              title={operationRoute.description}
              subtitle={`${operationRoute.operationType} operation (Step ${operationRoute.order})`}
            />
            <div className="mt-2 flex items-center gap-2">
              <Badge
                variant={
                  operationRoute.operationType === 'debit'
                    ? 'destructive'
                    : 'secondary'
                }
              >
                {operationRoute.operationType}
              </Badge>
              <Badge variant="outline">Order {operationRoute.order}</Badge>
              {operationRoute.conditions &&
                operationRoute.conditions.length > 0 && (
                  <Badge variant="secondary">
                    {operationRoute.conditions.length} conditions
                  </Badge>
                )}
            </div>
          </div>
        </div>
        <PageHeader.InfoTooltip subtitle="View and manage operation route details and configuration." />
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={handleDuplicate}>
            <Copy className="mr-2 h-4 w-4" />
            Duplicate
          </Button>
          <Button variant="outline" size="sm" onClick={handleEdit}>
            <Edit className="mr-2 h-4 w-4" />
            Edit
          </Button>
          <Button variant="destructive" size="sm" onClick={handleDelete}>
            <Trash2 className="mr-2 h-4 w-4" />
            Delete
          </Button>
        </div>
      </PageHeader.Root>

      <div className="flex-1 px-6 pb-6">
        <Tabs defaultValue="overview" className="space-y-6">
          <TabsList className="grid w-full grid-cols-4">
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="configuration">Configuration</TabsTrigger>
            <TabsTrigger value="conditions">Conditions</TabsTrigger>
            <TabsTrigger value="testing">Testing</TabsTrigger>
          </TabsList>

          <TabsContent value="overview" className="space-y-6">
            {/* Key Metrics */}
            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">
                    Operation Type
                  </CardTitle>
                  <ArrowUpDown className="h-4 w-4 text-muted-foreground" />
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold capitalize">
                    {operationRoute.operationType}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {operationRoute.operationType === 'debit'
                      ? 'Money outflow'
                      : 'Money inflow'}
                  </p>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">
                    Execution Order
                  </CardTitle>
                  <Activity className="h-4 w-4 text-muted-foreground" />
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">
                    #{operationRoute.order}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Execution sequence
                  </p>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">
                    Validation Status
                  </CardTitle>
                  {validationStatus === 'conditional' ? (
                    <AlertTriangle className="h-4 w-4 text-yellow-500" />
                  ) : (
                    <CheckCircle className="h-4 w-4 text-green-500" />
                  )}
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold capitalize">
                    {validationStatus}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {validationStatus === 'conditional'
                      ? 'Has conditions'
                      : 'No conditions'}
                  </p>
                </CardContent>
              </Card>
            </div>

            {/* Operation Flow */}
            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <Building className="h-5 w-5" />
                    Account Flow
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div>
                    <div className="mb-2 text-sm font-medium text-gray-600">
                      Source Account
                    </div>
                    <div className="flex items-center space-x-3 rounded-lg border border-red-200 bg-red-50 p-3">
                      <div className="h-2 w-2 rounded-full bg-red-500"></div>
                      <div>
                        <div className="font-medium">
                          {getAccountTypeCode(
                            operationRoute.sourceAccountTypeId
                          )}
                        </div>
                        <div className="text-sm text-gray-600">
                          {getAccountTypeName(
                            operationRoute.sourceAccountTypeId
                          )}
                        </div>
                      </div>
                    </div>
                  </div>

                  <div className="flex justify-center">
                    <ArrowUpDown className="h-6 w-6 text-gray-400" />
                  </div>

                  <div>
                    <div className="mb-2 text-sm font-medium text-gray-600">
                      Destination Account
                    </div>
                    <div className="flex items-center space-x-3 rounded-lg border border-green-200 bg-green-50 p-3">
                      <div className="h-2 w-2 rounded-full bg-green-500"></div>
                      <div>
                        <div className="font-medium">
                          {getAccountTypeCode(
                            operationRoute.destinationAccountTypeId
                          )}
                        </div>
                        <div className="text-sm text-gray-600">
                          {getAccountTypeName(
                            operationRoute.destinationAccountTypeId
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>Amount Configuration</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div>
                    <div className="text-sm font-medium text-gray-600">
                      Expression
                    </div>
                    <code className="mt-1 block rounded-lg bg-gray-100 p-3 font-mono text-sm">
                      {operationRoute.amount.expression}
                    </code>
                  </div>

                  <div>
                    <div className="text-sm font-medium text-gray-600">
                      Description
                    </div>
                    <div className="mt-1 text-sm leading-relaxed text-gray-700">
                      {operationRoute.amount.description}
                    </div>
                  </div>
                </CardContent>
              </Card>
            </div>
          </TabsContent>

          <TabsContent value="configuration">
            <Card>
              <CardHeader>
                <CardTitle>Operation Configuration</CardTitle>
                <CardDescription>
                  Detailed configuration settings for this operation route
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
                  <div className="space-y-4">
                    <div>
                      <label className="text-sm font-medium text-gray-600">
                        Description
                      </label>
                      <p className="mt-1 text-sm">
                        {operationRoute.description}
                      </p>
                    </div>

                    <div>
                      <label className="text-sm font-medium text-gray-600">
                        Operation Type
                      </label>
                      <div className="mt-1">
                        <Badge
                          variant={
                            operationRoute.operationType === 'debit'
                              ? 'destructive'
                              : 'secondary'
                          }
                        >
                          {operationRoute.operationType}
                        </Badge>
                      </div>
                    </div>

                    <div>
                      <label className="text-sm font-medium text-gray-600">
                        Execution Order
                      </label>
                      <p className="mt-1 text-sm">#{operationRoute.order}</p>
                    </div>
                  </div>

                  <div className="space-y-4">
                    <div>
                      <label className="text-sm font-medium text-gray-600">
                        Source Account Type ID
                      </label>
                      <code className="mt-1 block rounded bg-gray-100 p-2 font-mono text-sm">
                        {operationRoute.sourceAccountTypeId}
                      </code>
                    </div>

                    <div>
                      <label className="text-sm font-medium text-gray-600">
                        Destination Account Type ID
                      </label>
                      <code className="mt-1 block rounded bg-gray-100 p-2 font-mono text-sm">
                        {operationRoute.destinationAccountTypeId}
                      </code>
                    </div>
                  </div>
                </div>

                <div>
                  <label className="text-sm font-medium text-gray-600">
                    Amount Configuration
                  </label>
                  <pre className="mt-1 overflow-x-auto rounded-lg bg-gray-100 p-3 text-sm">
                    {JSON.stringify(operationRoute.amount, null, 2)}
                  </pre>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="conditions">
            <Card>
              <CardHeader>
                <CardTitle>Operation Conditions</CardTitle>
                <CardDescription>
                  Conditional logic that determines when this operation executes
                </CardDescription>
              </CardHeader>
              <CardContent>
                {operationRoute.conditions &&
                operationRoute.conditions.length > 0 ? (
                  <div className="space-y-4">
                    {operationRoute.conditions.map((condition, index) => (
                      <div
                        key={index}
                        className="rounded-lg border bg-yellow-50 p-4"
                      >
                        <div className="mb-3 flex items-center justify-between">
                          <Badge variant="outline">Condition {index + 1}</Badge>
                          <Badge
                            variant={
                              condition.operator === 'equals'
                                ? 'default'
                                : 'secondary'
                            }
                          >
                            {condition.operator}
                          </Badge>
                        </div>

                        <div className="grid grid-cols-1 gap-3 text-sm lg:grid-cols-3">
                          <div>
                            <div className="mb-1 font-medium text-gray-600">
                              Field
                            </div>
                            <code className="rounded bg-white p-1 text-xs">
                              {condition.field}
                            </code>
                          </div>
                          <div>
                            <div className="mb-1 font-medium text-gray-600">
                              Operator
                            </div>
                            <span className="capitalize">
                              {condition.operator}
                            </span>
                          </div>
                          <div>
                            <div className="mb-1 font-medium text-gray-600">
                              Value
                            </div>
                            <code className="rounded bg-white p-1 text-xs">
                              {condition.value}
                            </code>
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="py-8 text-center text-gray-500">
                    <CheckCircle className="mx-auto mb-4 h-12 w-12 text-gray-300" />
                    <p>No conditions configured</p>
                    <p className="text-sm">
                      This operation executes unconditionally
                    </p>
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="testing">
            <Card>
              <CardHeader>
                <CardTitle>Operation Testing</CardTitle>
                <CardDescription>
                  Test this operation with sample data and validate its behavior
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="py-8 text-center text-gray-500">
                  <Activity className="mx-auto mb-4 h-12 w-12 text-gray-300" />
                  <p>Testing interface will be implemented here</p>
                  <p className="text-sm">
                    Run simulations with different transaction amounts and
                    conditions
                  </p>
                  <Button className="mt-4" disabled>
                    Run Test Simulation
                  </Button>
                </div>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}
