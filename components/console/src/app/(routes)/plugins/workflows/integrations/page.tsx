'use client'

import React, { useState } from 'react'
import {
  Search,
  Plus,
  Filter,
  Grid3X3,
  List,
  Server,
  CheckCircle,
  XCircle,
  AlertCircle,
  Clock,
  Settings,
  TestTube,
  Eye,
  Edit,
  ExternalLink,
  Zap,
  Shield,
  Database,
  Globe,
  Users
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'

// Mock data for service integrations
const serviceIntegrations = [
  {
    id: 'midaz-onboarding',
    name: 'Midaz Onboarding',
    description:
      'Core onboarding service for organizations, ledgers, and accounts',
    status: 'healthy',
    type: 'Core Service',
    icon: Users,
    baseUrl: 'http://midaz-onboarding:3000',
    version: 'v1',
    endpoints: [
      {
        path: '/v1/organizations',
        method: 'GET',
        description: 'List organizations'
      },
      {
        path: '/v1/organizations',
        method: 'POST',
        description: 'Create organization'
      },
      { path: '/v1/ledgers', method: 'GET', description: 'List ledgers' },
      { path: '/v1/accounts', method: 'GET', description: 'List accounts' }
    ],
    usageCount: 1247,
    lastChecked: '2025-01-01T14:30:00Z',
    responseTime: '45ms'
  },
  {
    id: 'midaz-transaction',
    name: 'Midaz Transaction',
    description: 'Transaction processing and management service',
    status: 'healthy',
    type: 'Core Service',
    icon: Zap,
    baseUrl: 'http://midaz-transaction:3001',
    version: 'v1',
    endpoints: [
      {
        path: '/v1/transactions',
        method: 'GET',
        description: 'List transactions'
      },
      {
        path: '/v1/transactions',
        method: 'POST',
        description: 'Create transaction'
      },
      { path: '/v1/operations', method: 'GET', description: 'List operations' }
    ],
    usageCount: 2156,
    lastChecked: '2025-01-01T14:28:00Z',
    responseTime: '32ms'
  },
  {
    id: 'plugin-fees',
    name: 'Fees Service',
    description: 'Fee calculation and management plugin',
    status: 'healthy',
    type: 'Plugin',
    icon: Database,
    baseUrl: 'http://plugin-fees:4002',
    version: 'v1',
    endpoints: [
      {
        path: '/v1/fees/calculate',
        method: 'POST',
        description: 'Calculate fees'
      },
      { path: '/v1/packages', method: 'GET', description: 'List fee packages' },
      {
        path: '/v1/schedules',
        method: 'GET',
        description: 'List fee schedules'
      }
    ],
    usageCount: 892,
    lastChecked: '2025-01-01T14:25:00Z',
    responseTime: '28ms'
  },
  {
    id: 'plugin-identity',
    name: 'Identity Verification',
    description: 'KYC and identity verification plugin',
    status: 'warning',
    type: 'Plugin',
    icon: Shield,
    baseUrl: 'http://plugin-identity:4001',
    version: 'v1',
    endpoints: [
      { path: '/v1/verify', method: 'POST', description: 'Verify identity' },
      {
        path: '/v1/documents',
        method: 'POST',
        description: 'Upload documents'
      },
      {
        path: '/v1/status',
        method: 'GET',
        description: 'Check verification status'
      }
    ],
    usageCount: 234,
    lastChecked: '2025-01-01T14:20:00Z',
    responseTime: '156ms'
  },
  {
    id: 'plugin-crm',
    name: 'CRM Service',
    description: 'Customer relationship management plugin',
    status: 'healthy',
    type: 'Plugin',
    icon: Users,
    baseUrl: 'http://plugin-crm:4003',
    version: 'v1',
    endpoints: [
      { path: '/v1/customers', method: 'GET', description: 'List customers' },
      { path: '/v1/customers', method: 'POST', description: 'Create customer' },
      {
        path: '/v1/customers/{id}',
        method: 'PUT',
        description: 'Update customer'
      }
    ],
    usageCount: 567,
    lastChecked: '2025-01-01T14:27:00Z',
    responseTime: '38ms'
  },
  {
    id: 'external-payment-gateway',
    name: 'Payment Gateway',
    description: 'External payment processing gateway integration',
    status: 'error',
    type: 'External',
    icon: Globe,
    baseUrl: 'https://api.payment-gateway.com',
    version: 'v2',
    endpoints: [
      { path: '/v2/payments', method: 'POST', description: 'Process payment' },
      { path: '/v2/refunds', method: 'POST', description: 'Process refund' },
      { path: '/v2/webhooks', method: 'POST', description: 'Webhook endpoint' }
    ],
    usageCount: 89,
    lastChecked: '2025-01-01T14:15:00Z',
    responseTime: 'timeout'
  }
]

const statusColors = {
  healthy: 'text-green-600 bg-green-100 border-green-200',
  warning: 'text-yellow-600 bg-yellow-100 border-yellow-200',
  error: 'text-red-600 bg-red-100 border-red-200',
  unknown: 'text-gray-600 bg-gray-100 border-gray-200'
}

const statusIcons = {
  healthy: CheckCircle,
  warning: AlertCircle,
  error: XCircle,
  unknown: Clock
}

const categories = ['All', 'Core Service', 'Plugin', 'External']

export default function IntegrationsPage() {
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedCategory, setSelectedCategory] = useState('All')
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid')

  const filteredIntegrations = serviceIntegrations.filter((service) => {
    const matchesSearch =
      service.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      service.description.toLowerCase().includes(searchQuery.toLowerCase())
    const matchesCategory =
      selectedCategory === 'All' || service.type === selectedCategory
    return matchesSearch && matchesCategory
  })

  const getStatusIcon = (status: string) => {
    const IconComponent =
      statusIcons[status as keyof typeof statusIcons] || Clock
    return IconComponent
  }

  const healthyCount = serviceIntegrations.filter(
    (s) => s.status === 'healthy'
  ).length
  const warningCount = serviceIntegrations.filter(
    (s) => s.status === 'warning'
  ).length
  const errorCount = serviceIntegrations.filter(
    (s) => s.status === 'error'
  ).length

  return (
    <div className="space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Service Integrations</h1>
          <p className="text-muted-foreground">
            Manage service integrations, API endpoints, and monitor health
            status
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Button variant="outline" size="sm">
            <TestTube className="mr-2 h-4 w-4" />
            Test Integration
          </Button>
          <Button>
            <Plus className="mr-2 h-4 w-4" />
            Add Integration
          </Button>
        </div>
      </div>

      {/* Health Status Overview */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Total Services</p>
                <p className="text-2xl font-bold">
                  {serviceIntegrations.length}
                </p>
              </div>
              <Server className="h-8 w-8 text-muted-foreground" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Healthy</p>
                <p className="text-2xl font-bold text-green-600">
                  {healthyCount}
                </p>
              </div>
              <CheckCircle className="h-8 w-8 text-green-600" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Warnings</p>
                <p className="text-2xl font-bold text-yellow-600">
                  {warningCount}
                </p>
              </div>
              <AlertCircle className="h-8 w-8 text-yellow-600" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Errors</p>
                <p className="text-2xl font-bold text-red-600">{errorCount}</p>
              </div>
              <XCircle className="h-8 w-8 text-red-600" />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Filters and Search */}
      <div className="flex flex-wrap items-center gap-4">
        <div className="min-w-[300px] flex-1">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
            <Input
              placeholder="Search integrations by name or description..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-10"
            />
          </div>
        </div>

        <Select value={selectedCategory} onValueChange={setSelectedCategory}>
          <SelectTrigger className="w-[180px]">
            <Filter className="mr-2 h-4 w-4" />
            <SelectValue placeholder="Service Type" />
          </SelectTrigger>
          <SelectContent>
            {categories.map((category) => (
              <SelectItem key={category} value={category}>
                {category}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <div className="flex items-center rounded-lg border">
          <Button
            variant={viewMode === 'grid' ? 'default' : 'ghost'}
            size="sm"
            onClick={() => setViewMode('grid')}
            className="rounded-r-none"
          >
            <Grid3X3 className="h-4 w-4" />
          </Button>
          <Button
            variant={viewMode === 'list' ? 'default' : 'ghost'}
            size="sm"
            onClick={() => setViewMode('list')}
            className="rounded-l-none"
          >
            <List className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Integrations Display */}
      <Tabs defaultValue="services" className="w-full">
        <TabsList>
          <TabsTrigger value="services">Service Registry</TabsTrigger>
          <TabsTrigger value="endpoints">API Endpoints</TabsTrigger>
          <TabsTrigger value="testing">Integration Testing</TabsTrigger>
          <TabsTrigger value="monitoring">Health Monitoring</TabsTrigger>
        </TabsList>

        <TabsContent value="services" className="mt-6">
          {viewMode === 'grid' ? (
            <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
              {filteredIntegrations.map((service) => {
                const IconComponent = service.icon
                const StatusIcon = getStatusIcon(service.status)

                return (
                  <Card
                    key={service.id}
                    className="transition-shadow hover:shadow-md"
                  >
                    <CardHeader className="pb-3">
                      <div className="flex items-start justify-between">
                        <div className="flex items-center gap-3">
                          <div className="rounded-lg border border-blue-200 bg-blue-50 p-2">
                            <IconComponent className="h-5 w-5 text-blue-600" />
                          </div>
                          <div>
                            <CardTitle className="text-lg">
                              {service.name}
                            </CardTitle>
                            <div className="mt-1 flex items-center gap-2">
                              <Badge variant="secondary" className="text-xs">
                                {service.type}
                              </Badge>
                              <Badge
                                variant="outline"
                                className={`border text-xs ${statusColors[service.status as keyof typeof statusColors]}`}
                              >
                                <StatusIcon className="mr-1 h-3 w-3" />
                                {service.status}
                              </Badge>
                            </div>
                          </div>
                        </div>
                        <div className="flex items-center gap-1">
                          <Button variant="ghost" size="sm">
                            <TestTube className="h-4 w-4" />
                          </Button>
                          <Button variant="ghost" size="sm">
                            <Settings className="h-4 w-4" />
                          </Button>
                        </div>
                      </div>
                    </CardHeader>
                    <CardContent>
                      <CardDescription className="mb-4">
                        {service.description}
                      </CardDescription>

                      <div className="space-y-3">
                        <div className="flex items-center justify-between text-sm">
                          <span className="text-muted-foreground">
                            Base URL
                          </span>
                          <div className="flex items-center gap-1">
                            <code className="rounded bg-gray-100 px-2 py-1 text-xs">
                              {service.baseUrl}
                            </code>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-6 w-6 p-0"
                            >
                              <ExternalLink className="h-3 w-3" />
                            </Button>
                          </div>
                        </div>

                        <div className="flex items-center justify-between text-sm">
                          <span className="text-muted-foreground">
                            Endpoints
                          </span>
                          <span>{service.endpoints.length} endpoints</span>
                        </div>

                        <div className="flex items-center justify-between text-sm">
                          <span className="text-muted-foreground">Usage</span>
                          <span>{service.usageCount} requests</span>
                        </div>

                        <div className="flex items-center justify-between text-sm">
                          <span className="text-muted-foreground">
                            Response Time
                          </span>
                          <span
                            className={
                              service.responseTime === 'timeout'
                                ? 'text-red-600'
                                : ''
                            }
                          >
                            {service.responseTime}
                          </span>
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                )
              })}
            </div>
          ) : (
            <div className="space-y-4">
              {filteredIntegrations.map((service) => {
                const IconComponent = service.icon
                const StatusIcon = getStatusIcon(service.status)

                return (
                  <Card key={service.id}>
                    <CardContent className="p-4">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-4">
                          <div className="rounded-lg border border-blue-200 bg-blue-50 p-2">
                            <IconComponent className="h-5 w-5 text-blue-600" />
                          </div>
                          <div className="flex-1">
                            <div className="mb-1 flex items-center gap-3">
                              <h3 className="font-semibold">{service.name}</h3>
                              <Badge variant="secondary" className="text-xs">
                                {service.type}
                              </Badge>
                              <Badge
                                variant="outline"
                                className={`border text-xs ${statusColors[service.status as keyof typeof statusColors]}`}
                              >
                                <StatusIcon className="mr-1 h-3 w-3" />
                                {service.status}
                              </Badge>
                            </div>
                            <p className="mb-2 text-sm text-muted-foreground">
                              {service.description}
                            </p>
                            <div className="flex items-center gap-6 text-xs text-muted-foreground">
                              <span>{service.endpoints.length} endpoints</span>
                              <span>{service.usageCount} requests</span>
                              <span>Response: {service.responseTime}</span>
                              <code className="rounded bg-gray-100 px-2 py-1">
                                {service.baseUrl}
                              </code>
                            </div>
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          <Button variant="outline" size="sm">
                            <TestTube className="mr-1 h-4 w-4" />
                            Test
                          </Button>
                          <Button variant="ghost" size="sm">
                            <Eye className="h-4 w-4" />
                          </Button>
                          <Button variant="ghost" size="sm">
                            <Settings className="h-4 w-4" />
                          </Button>
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                )
              })}
            </div>
          )}

          {filteredIntegrations.length === 0 && (
            <div className="py-12 text-center">
              <Server className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
              <h3 className="mb-2 text-lg font-medium">
                No integrations found
              </h3>
              <p className="mb-4 text-muted-foreground">
                Try adjusting your search criteria or add a new integration.
              </p>
              <Button>
                <Plus className="mr-2 h-4 w-4" />
                Add Integration
              </Button>
            </div>
          )}
        </TabsContent>

        <TabsContent value="endpoints" className="mt-6">
          <Card>
            <CardHeader>
              <CardTitle>API Endpoint Management</CardTitle>
              <CardDescription>
                Manage and configure API endpoints for workflow integrations.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="py-8 text-center">
                <Settings className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
                <h3 className="mb-2 text-lg font-medium">
                  Endpoint Management Coming Soon
                </h3>
                <p className="text-muted-foreground">
                  Advanced endpoint configuration and management features are
                  under development.
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="testing" className="mt-6">
          <Card>
            <CardHeader>
              <CardTitle>Integration Testing</CardTitle>
              <CardDescription>
                Test service integrations and validate API connectivity before
                using in workflows.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="py-8 text-center">
                <TestTube className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
                <h3 className="mb-2 text-lg font-medium">
                  Integration Testing Coming Soon
                </h3>
                <p className="text-muted-foreground">
                  Comprehensive integration testing tools are being developed.
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="monitoring" className="mt-6">
          <Card>
            <CardHeader>
              <CardTitle>Health Monitoring</CardTitle>
              <CardDescription>
                Monitor service health, performance metrics, and availability
                status.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="py-8 text-center">
                <CheckCircle className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
                <h3 className="mb-2 text-lg font-medium">
                  Health Monitoring Coming Soon
                </h3>
                <p className="text-muted-foreground">
                  Real-time health monitoring and alerting features are in
                  development.
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
