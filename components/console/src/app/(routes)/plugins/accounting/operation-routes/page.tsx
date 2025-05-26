'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import {
  Plus,
  Search,
  Filter,
  MoreHorizontal,
  Edit,
  Eye,
  Copy,
  Trash2,
  ArrowUpDown,
  Building
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
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
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '@/components/page-header'

import {
  mockOperationRoutes,
  mockAccountTypes,
  type OperationRoute
} from '@/components/accounting/mock/transaction-route-mock-data'

export default function OperationRoutesPage() {
  const router = useRouter()
  const [searchTerm, setSearchTerm] = useState('')
  const [operationTypeFilter, setOperationTypeFilter] = useState<string>('all')
  const [accountTypeFilter, setAccountTypeFilter] = useState<string>('all')

  const filteredRoutes = mockOperationRoutes.filter((route) => {
    const matchesSearch =
      route.description.toLowerCase().includes(searchTerm.toLowerCase()) ||
      route.amount.description.toLowerCase().includes(searchTerm.toLowerCase())

    const matchesOperationType =
      operationTypeFilter === 'all' ||
      route.operationType === operationTypeFilter

    const matchesAccountType =
      accountTypeFilter === 'all' ||
      route.sourceAccountTypeId === accountTypeFilter ||
      route.destinationAccountTypeId === accountTypeFilter

    return matchesSearch && matchesOperationType && matchesAccountType
  })

  const debitOperations = mockOperationRoutes.filter(
    (route) => route.operationType === 'debit'
  ).length
  const creditOperations = mockOperationRoutes.filter(
    (route) => route.operationType === 'credit'
  ).length
  const conditionalOperations = mockOperationRoutes.filter(
    (route) => route.conditions && route.conditions.length > 0
  ).length

  const handleCreateOperation = () => {
    router.push('/plugins/accounting/operation-routes/create')
  }

  const handleViewOperation = (routeId: string) => {
    console.log('Viewing operation:', routeId)
    // In a real implementation, this would navigate to a details page
  }

  const handleEditOperation = (routeId: string) => {
    console.log('Editing operation:', routeId)
    // In a real implementation, this would open an edit dialog or navigate to edit page
  }

  const handleDuplicateOperation = (route: OperationRoute) => {
    console.log('Duplicating operation:', route.description)
    // In a real implementation, this would create a copy
  }

  const handleDeleteOperation = (routeId: string) => {
    console.log('Deleting operation:', routeId)
    // In a real implementation, this would show a confirmation dialog
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

  const renderOperationCard = (operation: OperationRoute) => (
    <Card
      key={operation.id}
      className="cursor-pointer transition-shadow hover:shadow-md"
    >
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="space-y-2">
            <div className="flex items-center space-x-2">
              <Badge
                variant={
                  operation.operationType === 'debit'
                    ? 'destructive'
                    : 'secondary'
                }
              >
                {operation.operationType}
              </Badge>
              <span className="text-xs text-muted-foreground">
                Step {operation.order}
              </span>
            </div>
            <CardTitle className="text-base">{operation.description}</CardTitle>
          </div>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                onClick={() => handleViewOperation(operation.id)}
              >
                <Eye className="mr-2 h-4 w-4" />
                View Details
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => handleEditOperation(operation.id)}
              >
                <Edit className="mr-2 h-4 w-4" />
                Edit Operation
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => handleDuplicateOperation(operation)}
              >
                <Copy className="mr-2 h-4 w-4" />
                Duplicate
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onClick={() => handleDeleteOperation(operation.id)}
                className="text-red-600"
              >
                <Trash2 className="mr-2 h-4 w-4" />
                Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          <div className="flex items-center justify-between text-sm">
            <div className="flex items-center space-x-2">
              <ArrowUpDown className="h-4 w-4 text-muted-foreground" />
              <span className="text-muted-foreground">Flow:</span>
            </div>
          </div>

          <div className="space-y-2 text-xs">
            <div className="flex items-center space-x-2">
              <Building className="h-3 w-3" />
              <span className="text-muted-foreground">From:</span>
              <span className="font-medium">
                {getAccountTypeCode(operation.sourceAccountTypeId)}
              </span>
            </div>
            <div className="flex items-center space-x-2">
              <Building className="h-3 w-3" />
              <span className="text-muted-foreground">To:</span>
              <span className="font-medium">
                {getAccountTypeCode(operation.destinationAccountTypeId)}
              </span>
            </div>
            <div className="flex items-center space-x-2">
              <span className="text-muted-foreground">Amount:</span>
              <span className="font-mono">{operation.amount.expression}</span>
            </div>
          </div>

          {operation.conditions && operation.conditions.length > 0 && (
            <div className="border-t pt-2">
              <Badge variant="outline" className="text-xs">
                {operation.conditions.length} conditions
              </Badge>
            </div>
          )}

          <div className="text-xs text-muted-foreground">
            {operation.amount.description}
          </div>
        </div>
      </CardContent>
    </Card>
  )

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <PageHeader.InfoTitle>Operation Routes</PageHeader.InfoTitle>
        <PageHeader.InfoTooltip>
          Manage individual operation mappings between account types with
          conditions and amount calculations.
        </PageHeader.InfoTooltip>
      </PageHeader.Root>

      {/* Stats Cards */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Total Operations
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {mockOperationRoutes.length}
            </div>
            <p className="text-xs text-muted-foreground">
              All operation routes
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Debit Operations
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-red-600">
              {debitOperations}
            </div>
            <p className="text-xs text-muted-foreground">Money outflows</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Credit Operations
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-600">
              {creditOperations}
            </div>
            <p className="text-xs text-muted-foreground">Money inflows</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Conditional</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-blue-600">
              {conditionalOperations}
            </div>
            <p className="text-xs text-muted-foreground">With conditions</p>
          </CardContent>
        </Card>
      </div>

      {/* Filters and Search */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Operation Management</CardTitle>
              <CardDescription>
                Create and manage individual operation route mappings
              </CardDescription>
            </div>
            <Button onClick={handleCreateOperation} className="gap-2">
              <Plus className="h-4 w-4" />
              Create Operation
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <div className="mb-6 flex items-center space-x-4">
            <div className="flex-1">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
                <Input
                  placeholder="Search operations by description or amount..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="pl-10"
                />
              </div>
            </div>
            <Select
              value={operationTypeFilter}
              onValueChange={setOperationTypeFilter}
            >
              <SelectTrigger className="w-[150px]">
                <Filter className="mr-2 h-4 w-4" />
                <SelectValue placeholder="Type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Types</SelectItem>
                <SelectItem value="debit">Debit</SelectItem>
                <SelectItem value="credit">Credit</SelectItem>
              </SelectContent>
            </Select>
            <Select
              value={accountTypeFilter}
              onValueChange={setAccountTypeFilter}
            >
              <SelectTrigger className="w-[200px]">
                <Filter className="mr-2 h-4 w-4" />
                <SelectValue placeholder="Account Type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Account Types</SelectItem>
                {mockAccountTypes.map((accountType) => (
                  <SelectItem key={accountType.id} value={accountType.id}>
                    {accountType.code}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <Tabs defaultValue="table" className="w-full">
            <TabsList>
              <TabsTrigger value="table">Table View</TabsTrigger>
              <TabsTrigger value="cards">Card View</TabsTrigger>
            </TabsList>

            <TabsContent value="table" className="mt-6">
              <div className="rounded-md border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Description</TableHead>
                      <TableHead>Type</TableHead>
                      <TableHead>Source Account</TableHead>
                      <TableHead>Destination Account</TableHead>
                      <TableHead>Amount</TableHead>
                      <TableHead>Order</TableHead>
                      <TableHead>Conditions</TableHead>
                      <TableHead className="w-[50px]"></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredRoutes.map((operation) => (
                      <TableRow
                        key={operation.id}
                        className="cursor-pointer hover:bg-muted/50"
                      >
                        <TableCell>
                          <div className="font-medium">
                            {operation.description}
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant={
                              operation.operationType === 'debit'
                                ? 'destructive'
                                : 'secondary'
                            }
                          >
                            {operation.operationType}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-sm">
                          <div className="space-y-1">
                            <div className="font-medium">
                              {getAccountTypeCode(
                                operation.sourceAccountTypeId
                              )}
                            </div>
                            <div className="text-xs text-muted-foreground">
                              {getAccountTypeName(
                                operation.sourceAccountTypeId
                              )}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell className="text-sm">
                          <div className="space-y-1">
                            <div className="font-medium">
                              {getAccountTypeCode(
                                operation.destinationAccountTypeId
                              )}
                            </div>
                            <div className="text-xs text-muted-foreground">
                              {getAccountTypeName(
                                operation.destinationAccountTypeId
                              )}
                            </div>
                          </div>
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
                        <TableCell className="text-center">
                          {operation.order}
                        </TableCell>
                        <TableCell>
                          {operation.conditions &&
                          operation.conditions.length > 0 ? (
                            <Badge variant="outline" className="text-xs">
                              {operation.conditions.length}
                            </Badge>
                          ) : (
                            <span className="text-xs text-muted-foreground">
                              None
                            </span>
                          )}
                        </TableCell>
                        <TableCell>
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button variant="ghost" size="sm">
                                <MoreHorizontal className="h-4 w-4" />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              <DropdownMenuItem
                                onClick={() =>
                                  handleViewOperation(operation.id)
                                }
                              >
                                <Eye className="mr-2 h-4 w-4" />
                                View Details
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                onClick={() =>
                                  handleEditOperation(operation.id)
                                }
                              >
                                <Edit className="mr-2 h-4 w-4" />
                                Edit Operation
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                onClick={() =>
                                  handleDuplicateOperation(operation)
                                }
                              >
                                <Copy className="mr-2 h-4 w-4" />
                                Duplicate
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem
                                onClick={() =>
                                  handleDeleteOperation(operation.id)
                                }
                                className="text-red-600"
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
              </div>
            </TabsContent>

            <TabsContent value="cards" className="mt-6">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
                {filteredRoutes.map(renderOperationCard)}
              </div>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
    </div>
  )
}
