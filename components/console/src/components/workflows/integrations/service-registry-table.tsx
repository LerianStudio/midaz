'use client'

import React from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  CheckCircle,
  XCircle,
  AlertCircle,
  Clock,
  TestTube,
  Eye,
  Settings,
  ExternalLink
} from 'lucide-react'

interface ServiceIntegration {
  id: string
  name: string
  description: string
  status: string
  type: string
  icon: React.ComponentType<{ className?: string }>
  baseUrl: string
  version: string
  endpoints: Array<{
    path: string
    method: string
    description: string
  }>
  usageCount: number
  lastChecked: string
  responseTime: string
}

interface ServiceRegistryTableProps {
  services: ServiceIntegration[]
  viewMode: 'grid' | 'list'
  onServiceTest?: (serviceId: string) => void
  onServiceView?: (serviceId: string) => void
  onServiceConfigure?: (serviceId: string) => void
}

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

export function ServiceRegistryTable({
  services,
  viewMode,
  onServiceTest,
  onServiceView,
  onServiceConfigure
}: ServiceRegistryTableProps) {
  const getStatusIcon = (status: string) => {
    const IconComponent =
      statusIcons[status as keyof typeof statusIcons] || Clock
    return IconComponent
  }

  if (viewMode === 'grid') {
    return (
      <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
        {services.map((service) => {
          const IconComponent = service.icon
          const StatusIcon = getStatusIcon(service.status)

          return (
            <Card
              key={service.id}
              className="transition-shadow hover:shadow-md"
            >
              <CardContent className="p-6">
                <div className="mb-4 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className="rounded-lg border border-blue-200 bg-blue-50 p-2">
                      <IconComponent className="h-5 w-5 text-blue-600" />
                    </div>
                    <div>
                      <h3 className="text-lg font-semibold">{service.name}</h3>
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
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => onServiceTest?.(service.id)}
                    >
                      <TestTube className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => onServiceConfigure?.(service.id)}
                    >
                      <Settings className="h-4 w-4" />
                    </Button>
                  </div>
                </div>

                <p className="mb-4 text-sm text-muted-foreground">
                  {service.description}
                </p>

                <div className="space-y-3">
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-muted-foreground">Base URL</span>
                    <div className="flex items-center gap-1">
                      <code className="rounded bg-gray-100 px-2 py-1 text-xs">
                        {service.baseUrl}
                      </code>
                      <Button variant="ghost" size="sm" className="h-6 w-6 p-0">
                        <ExternalLink className="h-3 w-3" />
                      </Button>
                    </div>
                  </div>

                  <div className="flex items-center justify-between text-sm">
                    <span className="text-muted-foreground">Endpoints</span>
                    <span>{service.endpoints.length} endpoints</span>
                  </div>

                  <div className="flex items-center justify-between text-sm">
                    <span className="text-muted-foreground">Usage</span>
                    <span>{service.usageCount} requests</span>
                  </div>

                  <div className="flex items-center justify-between text-sm">
                    <span className="text-muted-foreground">Response Time</span>
                    <span
                      className={
                        service.responseTime === 'timeout' ? 'text-red-600' : ''
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
    )
  }

  return (
    <div className="space-y-4">
      {services.map((service) => {
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
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => onServiceTest?.(service.id)}
                  >
                    <TestTube className="mr-1 h-4 w-4" />
                    Test
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => onServiceView?.(service.id)}
                  >
                    <Eye className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => onServiceConfigure?.(service.id)}
                  >
                    <Settings className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
