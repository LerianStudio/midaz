'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  DollarSign,
  Package,
  Calculator,
  TrendingUp,
  Activity,
  CreditCard,
  Percent,
  Users
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useRouter } from 'next/navigation'
import { Progress } from '@/components/ui/progress'

export default function FeesOverviewPage() {
  const intl = useIntl()
  const router = useRouter()

  // Mock data for overview metrics
  const metrics = {
    totalRevenue: 125430.5,
    activePackages: 12,
    transactionsProcessed: 3456,
    averageFeeRate: 2.35,
    waivedFees: 4320.0,
    topPackage: 'Standard Transaction Fees'
  }

  const recentActivity = [
    {
      id: 1,
      action: 'Package Created',
      detail: 'Premium Merchant Fees',
      time: '2 hours ago'
    },
    {
      id: 2,
      action: 'Fee Calculated',
      detail: 'Transaction #12345 - $45.20',
      time: '3 hours ago'
    },
    {
      id: 3,
      action: 'Package Updated',
      detail: 'International Transfer Fees',
      time: '5 hours ago'
    },
    {
      id: 4,
      action: 'Waiver Applied',
      detail: 'VIP Account #789',
      time: '1 day ago'
    }
  ]

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div>
        <h1 className="text-3xl font-bold">
          {intl.formatMessage({
            id: 'fees.overview.title',
            defaultMessage: 'Fees Management'
          })}
        </h1>
        <p className="mt-2 text-muted-foreground">
          {intl.formatMessage({
            id: 'fees.overview.subtitle',
            defaultMessage:
              'Configure and monitor transaction fees across your organization'
          })}
        </p>
      </div>

      {/* Quick Actions */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <Card
          className="cursor-pointer transition-shadow hover:shadow-md"
          onClick={() => router.push('/plugins/fees/packages/create')}
        >
          <CardContent className="p-6">
            <div className="flex items-center space-x-4">
              <div className="rounded-lg bg-primary/10 p-3">
                <Package className="h-6 w-6 text-primary" />
              </div>
              <div>
                <h3 className="font-semibold">Create Package</h3>
                <p className="text-sm text-muted-foreground">
                  Set up new fee rules
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card
          className="cursor-pointer transition-shadow hover:shadow-md"
          onClick={() => router.push('/plugins/fees/calculator')}
        >
          <CardContent className="p-6">
            <div className="flex items-center space-x-4">
              <div className="rounded-lg bg-green-100 p-3">
                <Calculator className="h-6 w-6 text-green-600" />
              </div>
              <div>
                <h3 className="font-semibold">Fee Calculator</h3>
                <p className="text-sm text-muted-foreground">
                  Test fee calculations
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card
          className="cursor-pointer transition-shadow hover:shadow-md"
          onClick={() => router.push('/plugins/fees/analytics')}
        >
          <CardContent className="p-6">
            <div className="flex items-center space-x-4">
              <div className="rounded-lg bg-purple-100 p-3">
                <TrendingUp className="h-6 w-6 text-purple-600" />
              </div>
              <div>
                <h3 className="font-semibold">Analytics</h3>
                <p className="text-sm text-muted-foreground">
                  View fee insights
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Metrics Overview */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Total Fee Revenue
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-2xl font-bold">
                  ${metrics.totalRevenue.toLocaleString()}
                </p>
                <p className="text-xs text-green-600">+12.5% from last month</p>
              </div>
              <DollarSign className="h-8 w-8 text-muted-foreground/20" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Active Packages
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-2xl font-bold">{metrics.activePackages}</p>
                <p className="text-xs text-muted-foreground">3 inactive</p>
              </div>
              <Package className="h-8 w-8 text-muted-foreground/20" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Transactions Processed
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-2xl font-bold">
                  {metrics.transactionsProcessed.toLocaleString()}
                </p>
                <p className="text-xs text-muted-foreground">This month</p>
              </div>
              <CreditCard className="h-8 w-8 text-muted-foreground/20" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Average Fee Rate
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-2xl font-bold">{metrics.averageFeeRate}%</p>
                <p className="text-xs text-muted-foreground">
                  Across all packages
                </p>
              </div>
              <Percent className="h-8 w-8 text-muted-foreground/20" />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Recent Activity and Top Packages */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Recent Activity */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Activity className="h-5 w-5" />
              Recent Activity
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {recentActivity.map((activity) => (
                <div
                  key={activity.id}
                  className="flex items-center justify-between border-b py-2 last:border-0"
                >
                  <div>
                    <p className="text-sm font-medium">{activity.action}</p>
                    <p className="text-sm text-muted-foreground">
                      {activity.detail}
                    </p>
                  </div>
                  <span className="text-xs text-muted-foreground">
                    {activity.time}
                  </span>
                </div>
              ))}
            </div>
            <Button
              variant="link"
              className="mt-4 p-0"
              onClick={() => router.push('/plugins/fees/packages')}
            >
              View all activity →
            </Button>
          </CardContent>
        </Card>

        {/* Top Performing Packages */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <TrendingUp className="h-5 w-5" />
              Top Performing Packages
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div>
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-sm font-medium">
                    {metrics.topPackage}
                  </span>
                  <span className="text-sm text-muted-foreground">45%</span>
                </div>
                <Progress value={45} className="h-2" />
              </div>
              <div>
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-sm font-medium">
                    Premium Merchant Fees
                  </span>
                  <span className="text-sm text-muted-foreground">30%</span>
                </div>
                <Progress value={30} className="h-2" />
              </div>
              <div>
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-sm font-medium">
                    International Transfer Fees
                  </span>
                  <span className="text-sm text-muted-foreground">15%</span>
                </div>
                <Progress value={15} className="h-2" />
              </div>
              <div>
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-sm font-medium">Other Packages</span>
                  <span className="text-sm text-muted-foreground">10%</span>
                </div>
                <Progress value={10} className="h-2" />
              </div>
            </div>
            <Button
              variant="link"
              className="mt-4 p-0"
              onClick={() => router.push('/plugins/fees/analytics')}
            >
              View detailed analytics →
            </Button>
          </CardContent>
        </Card>
      </div>

      {/* Waived Fees Info */}
      <Card className="border-amber-200 bg-amber-50">
        <CardContent className="p-6">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <Users className="h-8 w-8 text-amber-600" />
              <div>
                <h3 className="font-semibold text-amber-900">
                  Waived Fees This Month
                </h3>
                <p className="text-sm text-amber-700">
                  ${metrics.waivedFees.toLocaleString()} in fees waived for VIP
                  accounts
                </p>
              </div>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => router.push('/plugins/fees/packages')}
            >
              Manage Waivers
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
