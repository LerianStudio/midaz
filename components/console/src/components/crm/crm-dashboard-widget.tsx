'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { useRouter } from 'next/navigation'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Users,
  Building2,
  CreditCard,
  TrendingUp,
  ExternalLink,
  Plus,
  UserPlus
} from 'lucide-react'
import {
  generateMockCustomers,
  generateMockAliases
} from './customers/customer-mock-data'
import { CustomerType } from './customers/customer-types'

export const CRMDashboardWidget: React.FC = () => {
  const intl = useIntl()
  const router = useRouter()

  // Generate mock data for dashboard metrics
  const customers = generateMockCustomers(50)
  const aliases = generateMockAliases(150)

  const metrics = {
    totalCustomers: customers.length,
    naturalPersons: customers.filter(
      (c) => c.type === CustomerType.NATURAL_PERSON
    ).length,
    legalPersons: customers.filter((c) => c.type === CustomerType.LEGAL_PERSON)
      .length,
    activeCustomers: customers.filter((c) => c.status === 'active').length,
    totalAliases: aliases.length,
    newThisMonth: customers.filter((c) => {
      const createdDate = new Date(c.createdAt)
      const now = new Date()
      return (
        createdDate.getMonth() === now.getMonth() &&
        createdDate.getFullYear() === now.getFullYear()
      )
    }).length,
    recentCustomers: customers.slice(0, 3)
  }

  const getCustomerTypeIcon = (type: CustomerType) => {
    return type === CustomerType.NATURAL_PERSON ? (
      <Users className="h-4 w-4 text-blue-600" />
    ) : (
      <Building2 className="h-4 w-4 text-purple-600" />
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">
            {intl.formatMessage({
              id: 'crm.dashboard.title',
              defaultMessage: 'Customer Relationship Management'
            })}
          </h2>
          <p className="text-sm text-muted-foreground">
            {intl.formatMessage({
              id: 'crm.dashboard.subtitle',
              defaultMessage: 'Overview of your customer base and relationships'
            })}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => router.push('/plugins/crm')}
        >
          {intl.formatMessage({
            id: 'crm.dashboard.viewAll',
            defaultMessage: 'View CRM'
          })}
          <ExternalLink className="ml-2 h-4 w-4" />
        </Button>
      </div>

      {/* Metrics Grid */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center space-x-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/10">
                <Users className="h-4 w-4 text-primary" />
              </div>
              <div>
                <div className="text-2xl font-bold">
                  {metrics.totalCustomers}
                </div>
                <p className="text-xs text-muted-foreground">Total Customers</p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center space-x-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-green-100 dark:bg-green-900">
                <TrendingUp className="h-4 w-4 text-green-600" />
              </div>
              <div>
                <div className="text-2xl font-bold">
                  {metrics.activeCustomers}
                </div>
                <p className="text-xs text-muted-foreground">
                  Active Customers
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center space-x-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-blue-100 dark:bg-blue-900">
                <CreditCard className="h-4 w-4 text-blue-600" />
              </div>
              <div>
                <div className="text-2xl font-bold">{metrics.totalAliases}</div>
                <p className="text-xs text-muted-foreground">Banking Aliases</p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center space-x-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-full bg-orange-100 dark:bg-orange-900">
                <Plus className="h-4 w-4 text-orange-600" />
              </div>
              <div>
                <div className="text-2xl font-bold">{metrics.newThisMonth}</div>
                <p className="text-xs text-muted-foreground">New This Month</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Customer Types Breakdown */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Customer Types</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-2">
                <Users className="h-4 w-4 text-blue-600" />
                <span className="text-sm">Individual Customers</span>
              </div>
              <div className="flex items-center space-x-2">
                <span className="font-medium">{metrics.naturalPersons}</span>
                <Badge variant="outline" className="text-xs">
                  {Math.round(
                    (metrics.naturalPersons / metrics.totalCustomers) * 100
                  )}
                  %
                </Badge>
              </div>
            </div>

            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-2">
                <Building2 className="h-4 w-4 text-purple-600" />
                <span className="text-sm">Corporate Customers</span>
              </div>
              <div className="flex items-center space-x-2">
                <span className="font-medium">{metrics.legalPersons}</span>
                <Badge variant="outline" className="text-xs">
                  {Math.round(
                    (metrics.legalPersons / metrics.totalCustomers) * 100
                  )}
                  %
                </Badge>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-base">Recent Customers</CardTitle>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => router.push('/plugins/crm/customers')}
            >
              View All
            </Button>
          </CardHeader>
          <CardContent className="space-y-3">
            {metrics.recentCustomers.map((customer) => (
              <div
                key={customer.id}
                className="flex cursor-pointer items-center space-x-3 rounded-lg p-2 transition-colors hover:bg-muted/50"
                onClick={() =>
                  router.push(`/plugins/crm/customers/${customer.id}`)
                }
              >
                <div className="flex-shrink-0">
                  {getCustomerTypeIcon(customer.type)}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">
                    {customer.name}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {customer.contact.primaryEmail}
                  </p>
                </div>
                <Badge
                  variant={
                    customer.status === 'active' ? 'default' : 'secondary'
                  }
                  className="text-xs"
                >
                  {customer.status}
                </Badge>
              </div>
            ))}
          </CardContent>
        </Card>
      </div>

      {/* Quick Actions */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Quick Actions</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => router.push('/plugins/crm/customers/create')}
            >
              <UserPlus className="mr-2 h-4 w-4" />
              Add Customer
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => router.push('/plugins/crm/customers')}
            >
              <Users className="mr-2 h-4 w-4" />
              Manage Customers
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => router.push('/plugins/crm')}
            >
              <CreditCard className="mr-2 h-4 w-4" />
              View CRM Dashboard
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
