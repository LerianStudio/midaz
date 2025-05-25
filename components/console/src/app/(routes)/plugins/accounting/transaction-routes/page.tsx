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
  Trash2
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
  mockTransactionRoutes,
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

export default function TransactionRoutesPage() {
  const router = useRouter()
  const [searchTerm, setSearchTerm] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [templateTypeFilter, setTemplateTypeFilter] = useState<string>('all')

  const filteredRoutes = mockTransactionRoutes.filter((route) => {
    const matchesSearch =
      route.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      route.description.toLowerCase().includes(searchTerm.toLowerCase()) ||
      route.tags.some((tag) =>
        tag.toLowerCase().includes(searchTerm.toLowerCase())
      )

    const matchesStatus =
      statusFilter === 'all' || route.status === statusFilter
    const matchesTemplateType =
      templateTypeFilter === 'all' || route.templateType === templateTypeFilter

    return matchesSearch && matchesStatus && matchesTemplateType
  })

  const activeRoutes = mockTransactionRoutes.filter(
    (route) => route.status === 'active'
  ).length
  const draftRoutes = mockTransactionRoutes.filter(
    (route) => route.status === 'draft'
  ).length

  const handleCreateRoute = () => {
    router.push('/plugins/accounting/transaction-routes/create')
  }

  const handleViewRoute = (routeId: string) => {
    router.push(`/plugins/accounting/transaction-routes/${routeId}`)
  }

  const handleEditRoute = (routeId: string) => {
    router.push(`/plugins/accounting/transaction-routes/${routeId}/designer`)
  }

  const handleDuplicateRoute = (route: TransactionRoute) => {
    // In a real implementation, this would create a copy and redirect to edit
    console.log('Duplicating route:', route.name)
  }

  const handleDeleteRoute = (routeId: string) => {
    // In a real implementation, this would show a confirmation dialog
    console.log('Deleting route:', routeId)
  }

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric'
    })
  }

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <PageHeader.InfoTitle>Transaction Routes</PageHeader.InfoTitle>
        <PageHeader.InfoTooltip>
          Configure transaction routing rules and operation mappings for
          automated transaction processing.
        </PageHeader.InfoTooltip>
      </PageHeader.Root>

      {/* Stats Cards */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Routes</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {mockTransactionRoutes.length}
            </div>
            <p className="text-xs text-muted-foreground">
              All transaction routes
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Active Routes</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-600">
              {activeRoutes}
            </div>
            <p className="text-xs text-muted-foreground">Currently in use</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Draft Routes</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-yellow-600">
              {draftRoutes}
            </div>
            <p className="text-xs text-muted-foreground">Being developed</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Avg Operations
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {Math.round(
                mockTransactionRoutes.reduce(
                  (sum, route) => sum + route.operationRoutes.length,
                  0
                ) / mockTransactionRoutes.length
              )}
            </div>
            <p className="text-xs text-muted-foreground">Per route</p>
          </CardContent>
        </Card>
      </div>

      {/* Filters and Search */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Route Management</CardTitle>
              <CardDescription>
                Create and manage transaction routing configurations
              </CardDescription>
            </div>
            <Button onClick={handleCreateRoute} className="gap-2">
              <Plus className="h-4 w-4" />
              Create Route
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <div className="mb-6 flex items-center space-x-4">
            <div className="flex-1">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
                <Input
                  placeholder="Search routes by name, description, or tags..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="pl-10"
                />
              </div>
            </div>
            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger className="w-[150px]">
                <Filter className="mr-2 h-4 w-4" />
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Status</SelectItem>
                <SelectItem value="active">Active</SelectItem>
                <SelectItem value="draft">Draft</SelectItem>
                <SelectItem value="deprecated">Deprecated</SelectItem>
              </SelectContent>
            </Select>
            <Select
              value={templateTypeFilter}
              onValueChange={setTemplateTypeFilter}
            >
              <SelectTrigger className="w-[150px]">
                <Filter className="mr-2 h-4 w-4" />
                <SelectValue placeholder="Type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Types</SelectItem>
                <SelectItem value="transfer">Transfer</SelectItem>
                <SelectItem value="payment">Payment</SelectItem>
                <SelectItem value="adjustment">Adjustment</SelectItem>
                <SelectItem value="fee">Fee</SelectItem>
                <SelectItem value="refund">Refund</SelectItem>
                <SelectItem value="custom">Custom</SelectItem>
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
                      <TableHead>Name</TableHead>
                      <TableHead>Type</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Operations</TableHead>
                      <TableHead>Version</TableHead>
                      <TableHead>Updated</TableHead>
                      <TableHead className="w-[50px]"></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredRoutes.map((route) => (
                      <TableRow
                        key={route.id}
                        className="cursor-pointer hover:bg-muted/50"
                      >
                        <TableCell>
                          <div className="space-y-1">
                            <div className="font-medium">{route.name}</div>
                            <div className="text-sm text-muted-foreground">
                              {route.description}
                            </div>
                            <div className="flex flex-wrap gap-1">
                              {route.tags.slice(0, 3).map((tag) => (
                                <Badge
                                  key={tag}
                                  variant="outline"
                                  className="text-xs"
                                >
                                  {tag}
                                </Badge>
                              ))}
                              {route.tags.length > 3 && (
                                <Badge variant="outline" className="text-xs">
                                  +{route.tags.length - 3}
                                </Badge>
                              )}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge
                            className={templateTypeColors[route.templateType]}
                          >
                            {route.templateType}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <Badge className={statusColors[route.status]}>
                            {route.status}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-center">
                          {route.operationRoutes.length}
                        </TableCell>
                        <TableCell>{route.version}</TableCell>
                        <TableCell>{formatDate(route.updatedAt)}</TableCell>
                        <TableCell>
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button variant="ghost" size="sm">
                                <MoreHorizontal className="h-4 w-4" />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              <DropdownMenuItem
                                onClick={() => handleViewRoute(route.id)}
                              >
                                <Eye className="mr-2 h-4 w-4" />
                                View Details
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                onClick={() => handleEditRoute(route.id)}
                              >
                                <Edit className="mr-2 h-4 w-4" />
                                Edit Route
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                onClick={() => handleDuplicateRoute(route)}
                              >
                                <Copy className="mr-2 h-4 w-4" />
                                Duplicate
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem
                                onClick={() => handleDeleteRoute(route.id)}
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
                {filteredRoutes.map((route) => (
                  <Card
                    key={route.id}
                    className="cursor-pointer transition-shadow hover:shadow-md"
                  >
                    <CardHeader className="pb-3">
                      <div className="flex items-start justify-between">
                        <div className="space-y-1">
                          <CardTitle className="text-lg">
                            {route.name}
                          </CardTitle>
                          <CardDescription className="text-sm">
                            {route.description}
                          </CardDescription>
                        </div>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="sm">
                              <MoreHorizontal className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem
                              onClick={() => handleViewRoute(route.id)}
                            >
                              <Eye className="mr-2 h-4 w-4" />
                              View Details
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              onClick={() => handleEditRoute(route.id)}
                            >
                              <Edit className="mr-2 h-4 w-4" />
                              Edit Route
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              onClick={() => handleDuplicateRoute(route)}
                            >
                              <Copy className="mr-2 h-4 w-4" />
                              Duplicate
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem
                              onClick={() => handleDeleteRoute(route.id)}
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
                        <div className="flex items-center justify-between">
                          <Badge
                            className={templateTypeColors[route.templateType]}
                          >
                            {route.templateType}
                          </Badge>
                          <Badge className={statusColors[route.status]}>
                            {route.status}
                          </Badge>
                        </div>
                        <div className="text-sm text-muted-foreground">
                          {route.operationRoutes.length} operations • v
                          {route.version}
                        </div>
                        <div className="flex flex-wrap gap-1">
                          {route.tags.slice(0, 3).map((tag) => (
                            <Badge
                              key={tag}
                              variant="outline"
                              className="text-xs"
                            >
                              {tag}
                            </Badge>
                          ))}
                          {route.tags.length > 3 && (
                            <Badge variant="outline" className="text-xs">
                              +{route.tags.length - 3}
                            </Badge>
                          )}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          Updated {formatDate(route.updatedAt)}
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
    </div>
  )
}
