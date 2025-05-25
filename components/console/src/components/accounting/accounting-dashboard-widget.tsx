'use client'

import { useState } from 'react'
import {
  Calculator,
  Users,
  GitBranch,
  CheckCircle,
  ArrowRight
} from 'lucide-react'
import Link from 'next/link'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

// Import mock data to show real statistics
import { mockAnalyticsData } from '@/core/domain/mock-data/accounting-mock-data'

export interface AccountingDashboardWidgetProps {
  className?: string
}

export function AccountingDashboardWidget({
  className
}: AccountingDashboardWidgetProps) {
  const analytics = mockAnalyticsData.overview

  const quickActions = [
    {
      title: 'Create Account Type',
      description: 'Add new account type to chart',
      href: '/plugins/accounting/account-types/create',
      icon: <Users className="h-4 w-4" />,
      color: 'bg-blue-100 text-blue-600'
    },
    {
      title: 'Design Transaction Route',
      description: 'Create accounting template',
      href: '/plugins/accounting/transaction-routes/create',
      icon: <GitBranch className="h-4 w-4" />,
      color: 'bg-green-100 text-green-600'
    },
    {
      title: 'View Compliance',
      description: 'Monitor compliance status',
      href: '/plugins/accounting/compliance',
      icon: <CheckCircle className="h-4 w-4" />,
      color: 'bg-purple-100 text-purple-600'
    }
  ]

  return (
    <Card className={className}>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <div className="rounded-lg bg-orange-100 p-2">
              <Calculator className="h-5 w-5 text-orange-600" />
            </div>
            <div>
              <CardTitle className="text-lg">Accounting & Compliance</CardTitle>
              <CardDescription>
                Chart of accounts and financial governance
              </CardDescription>
            </div>
          </div>
          <Button asChild variant="ghost" size="sm">
            <Link href="/plugins/accounting">
              View All
              <ArrowRight className="ml-2 h-4 w-4" />
            </Link>
          </Button>
        </div>
      </CardHeader>

      <CardContent className="space-y-6">
        {/* Quick Stats */}
        <div className="grid grid-cols-4 gap-3">
          <div className="space-y-1 text-center">
            <div className="text-2xl font-bold text-blue-600">
              {analytics.totalAccountTypes}
            </div>
            <div className="text-xs text-muted-foreground">Account Types</div>
          </div>
          <div className="space-y-1 text-center">
            <div className="text-2xl font-bold text-green-600">
              {analytics.totalTransactionRoutes}
            </div>
            <div className="text-xs text-muted-foreground">
              Transaction Routes
            </div>
          </div>
          <div className="space-y-1 text-center">
            <div className="text-2xl font-bold text-purple-600">
              {analytics.totalOperationRoutes}
            </div>
            <div className="text-xs text-muted-foreground">Operations</div>
          </div>
          <div className="space-y-1 text-center">
            <div className="text-2xl font-bold text-orange-600">
              {analytics.complianceScore}%
            </div>
            <div className="text-xs text-muted-foreground">Compliance</div>
          </div>
        </div>

        {/* Status Overview */}
        <div className="space-y-2">
          <div className="flex items-center justify-between text-sm">
            <span className="text-muted-foreground">Account Types</span>
            <div className="flex items-center space-x-2">
              <Badge variant="default" className="text-xs">
                {analytics.activeAccountTypes} active
              </Badge>
              <Badge variant="secondary" className="text-xs">
                {analytics.totalAccountTypes - analytics.activeAccountTypes}{' '}
                inactive
              </Badge>
            </div>
          </div>

          <div className="flex items-center justify-between text-sm">
            <span className="text-muted-foreground">Transaction Routes</span>
            <div className="flex items-center space-x-2">
              <Badge variant="default" className="text-xs">
                {analytics.activeTransactionRoutes} active
              </Badge>
              <Badge variant="outline" className="text-xs">
                {analytics.totalTransactionRoutes -
                  analytics.activeTransactionRoutes}{' '}
                draft
              </Badge>
            </div>
          </div>

          <div className="flex items-center justify-between text-sm">
            <span className="text-muted-foreground">Monthly Usage</span>
            <span className="font-medium">
              {analytics.monthlyUsage.toLocaleString()}
            </span>
          </div>
        </div>

        {/* Quick Actions */}
        <div className="space-y-2">
          <div className="text-sm font-medium">Quick Actions</div>
          <div className="grid gap-2">
            {quickActions.map((action) => (
              <Button
                key={action.title}
                asChild
                variant="ghost"
                size="sm"
                className="h-auto justify-start p-2"
              >
                <Link
                  href={action.href}
                  className="flex items-center space-x-3"
                >
                  <div className={`rounded p-1 ${action.color}`}>
                    {action.icon}
                  </div>
                  <div className="flex-1 text-left">
                    <div className="font-medium">{action.title}</div>
                    <div className="text-xs text-muted-foreground">
                      {action.description}
                    </div>
                  </div>
                  <ArrowRight className="h-3 w-3 text-muted-foreground" />
                </Link>
              </Button>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
