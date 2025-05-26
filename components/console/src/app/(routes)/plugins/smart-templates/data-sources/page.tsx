import React from 'react'
import { Metadata } from 'next'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Database,
  Plus,
  Settings,
  CheckCircle,
  AlertCircle
} from 'lucide-react'

export const metadata: Metadata = {
  title: 'Data Sources - Smart Templates',
  description: 'Manage template data sources and connections'
}

const mockDataSources = [
  {
    id: 'ds-1',
    name: 'Midaz Core API',
    type: 'REST API',
    status: 'connected',
    description: 'Transaction and account data from Midaz core services',
    lastSync: '5 minutes ago'
  },
  {
    id: 'ds-2',
    name: 'Analytics Database',
    type: 'PostgreSQL',
    status: 'connected',
    description: 'Aggregated financial metrics and analytics data',
    lastSync: '1 hour ago'
  },
  {
    id: 'ds-3',
    name: 'External Bank APIs',
    type: 'REST API',
    status: 'error',
    description: 'Real-time balance and transaction updates',
    lastSync: '2 hours ago'
  },
  {
    id: 'ds-4',
    name: 'User Management Service',
    type: 'gRPC',
    status: 'connected',
    description: 'Customer profiles and user information',
    lastSync: '30 minutes ago'
  }
]

export default function DataSourcesPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Data Sources</h1>
          <p className="text-muted-foreground">
            Manage connections to external data sources for your templates
          </p>
        </div>
        <Button className="flex items-center space-x-2">
          <Plus className="h-4 w-4" />
          <span>Add Data Source</span>
        </Button>
      </div>

      <div className="grid gap-4">
        {mockDataSources.map((source) => (
          <Card key={source.id}>
            <CardContent className="p-6">
              <div className="flex items-start justify-between">
                <div className="flex items-start space-x-4">
                  <Database className="mt-1 h-8 w-8 text-blue-500" />
                  <div className="flex-1">
                    <div className="mb-2 flex items-center space-x-3">
                      <h3 className="font-semibold">{source.name}</h3>
                      <Badge variant="outline">{source.type}</Badge>
                      <Badge
                        className={
                          source.status === 'connected'
                            ? 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200'
                            : 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200'
                        }
                      >
                        <div className="flex items-center space-x-1">
                          {source.status === 'connected' ? (
                            <CheckCircle className="h-3 w-3" />
                          ) : (
                            <AlertCircle className="h-3 w-3" />
                          )}
                          <span>{source.status}</span>
                        </div>
                      </Badge>
                    </div>
                    <p className="mb-2 text-muted-foreground">
                      {source.description}
                    </p>
                    <p className="text-sm text-muted-foreground">
                      Last synced: {source.lastSync}
                    </p>
                  </div>
                </div>
                <div className="flex items-center space-x-2">
                  <Button variant="outline" size="sm">
                    <Settings className="mr-2 h-4 w-4" />
                    Configure
                  </Button>
                  <Button variant="outline" size="sm">
                    Test Connection
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Available Integrations</CardTitle>
          <CardDescription>
            Connect to additional data sources for your templates
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            {[
              {
                name: 'MongoDB',
                type: 'Database',
                description: 'NoSQL document database'
              },
              {
                name: 'Redis',
                type: 'Cache',
                description: 'In-memory data structure store'
              },
              {
                name: 'Webhook',
                type: 'API',
                description: 'Real-time event notifications'
              },
              {
                name: 'CSV Upload',
                type: 'File',
                description: 'Static data file uploads'
              },
              {
                name: 'GraphQL API',
                type: 'API',
                description: 'Flexible query interface'
              },
              {
                name: 'Message Queue',
                type: 'Stream',
                description: 'Event-driven data flow'
              }
            ].map((integration, index) => (
              <Card
                key={index}
                className="cursor-pointer transition-shadow hover:shadow-md"
              >
                <CardContent className="p-4">
                  <div className="space-y-2">
                    <div className="flex items-center justify-between">
                      <h4 className="font-medium">{integration.name}</h4>
                      <Badge variant="outline" className="text-xs">
                        {integration.type}
                      </Badge>
                    </div>
                    <p className="text-sm text-muted-foreground">
                      {integration.description}
                    </p>
                    <Button variant="ghost" size="sm" className="w-full">
                      Configure
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
