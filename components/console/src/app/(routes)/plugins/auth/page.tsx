'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Shield, Users, Key, Settings, BarChart3 } from 'lucide-react'

export default function AuthPage() {
  const intl = useIntl()

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">
          Authentication & Authorization
        </h1>
        <p className="text-muted-foreground">
          User authentication, role-based access control, and security
          management with JWT integration.
        </p>
      </div>

      {/* Quick Stats */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card className="p-6">
          <div className="flex items-center">
            <Users className="h-8 w-8 text-blue-600" />
            <div className="ml-4">
              <p className="text-sm font-medium text-muted-foreground">
                Active Users
              </p>
              <p className="text-2xl font-bold">1,234</p>
            </div>
          </div>
        </Card>
        <Card className="p-6">
          <div className="flex items-center">
            <Shield className="h-8 w-8 text-green-600" />
            <div className="ml-4">
              <p className="text-sm font-medium text-muted-foreground">
                Security Score
              </p>
              <p className="text-2xl font-bold">98%</p>
            </div>
          </div>
        </Card>
        <Card className="p-6">
          <div className="flex items-center">
            <Key className="h-8 w-8 text-orange-600" />
            <div className="ml-4">
              <p className="text-sm font-medium text-muted-foreground">
                Active Tokens
              </p>
              <p className="text-2xl font-bold">567</p>
            </div>
          </div>
        </Card>
        <Card className="p-6">
          <div className="flex items-center">
            <Settings className="h-8 w-8 text-purple-600" />
            <div className="ml-4">
              <p className="text-sm font-medium text-muted-foreground">Roles</p>
              <p className="text-2xl font-bold">12</p>
            </div>
          </div>
        </Card>
      </div>

      {/* Quick Actions */}
      <div className="grid gap-6 md:grid-cols-2">
        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-lg font-semibold">User Management</h3>
              <p className="text-sm text-muted-foreground">
                Manage user accounts, roles, and permissions
              </p>
            </div>
            <Button>
              <Users className="mr-2 h-4 w-4" />
              Manage Users
            </Button>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-lg font-semibold">Security Settings</h3>
              <p className="text-sm text-muted-foreground">
                Configure authentication and security policies
              </p>
            </div>
            <Button>
              <Shield className="mr-2 h-4 w-4" />
              Security Config
            </Button>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-lg font-semibold">Token Management</h3>
              <p className="text-sm text-muted-foreground">
                Manage JWT tokens, API keys, and access credentials
              </p>
            </div>
            <Button>
              <Key className="mr-2 h-4 w-4" />
              Manage Tokens
            </Button>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-lg font-semibold">Analytics</h3>
              <p className="text-sm text-muted-foreground">
                View authentication logs and security metrics
              </p>
            </div>
            <Button>
              <BarChart3 className="mr-2 h-4 w-4" />
              View Analytics
            </Button>
          </div>
        </Card>
      </div>

      {/* Recent Activity */}
      <Card>
        <div className="p-6">
          <h3 className="mb-4 text-lg font-semibold">
            Recent Authentication Activity
          </h3>
          <div className="space-y-4">
            <div className="flex items-center justify-between border-b pb-2">
              <div>
                <p className="font-medium">User Login</p>
                <p className="text-sm text-muted-foreground">
                  john.doe@example.com
                </p>
              </div>
              <span className="text-sm text-green-600">Success</span>
            </div>
            <div className="flex items-center justify-between border-b pb-2">
              <div>
                <p className="font-medium">Failed Login Attempt</p>
                <p className="text-sm text-muted-foreground">
                  suspicious.user@example.com
                </p>
              </div>
              <span className="text-sm text-red-600">Failed</span>
            </div>
            <div className="flex items-center justify-between border-b pb-2">
              <div>
                <p className="font-medium">Token Refresh</p>
                <p className="text-sm text-muted-foreground">api-service-01</p>
              </div>
              <span className="text-sm text-blue-600">Success</span>
            </div>
          </div>
        </div>
      </Card>
    </div>
  )
}
