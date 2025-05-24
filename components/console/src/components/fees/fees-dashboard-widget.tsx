'use client'

import React from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Receipt, TrendingUp, Package, DollarSign } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useRouter } from 'next/navigation'
import { Progress } from '@/components/ui/progress'
import { generateMockAnalytics } from './mock/fee-mock-data'

export function FeesDashboardWidget() {
  const router = useRouter()
  const analytics = generateMockAnalytics()

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center justify-between">
          <span className="flex items-center gap-2">
            <Receipt className="h-5 w-5" />
            Fee Management
          </span>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => router.push('/plugins/fees')}
          >
            View Details →
          </Button>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="mb-4 grid grid-cols-2 gap-4">
          <div className="space-y-1">
            <p className="text-sm text-muted-foreground">Total Revenue</p>
            <p className="text-2xl font-bold">
              $
              {analytics.totalRevenue.toLocaleString(undefined, {
                minimumFractionDigits: 2,
                maximumFractionDigits: 2
              })}
            </p>
            <p className="flex items-center gap-1 text-xs text-green-600">
              <TrendingUp className="h-3 w-3" />
              +15.3% from last month
            </p>
          </div>
          <div className="space-y-1">
            <p className="text-sm text-muted-foreground">Active Packages</p>
            <p className="text-2xl font-bold">
              {analytics.packageBreakdown.length}
            </p>
            <p className="flex items-center gap-1 text-xs text-muted-foreground">
              <Package className="h-3 w-3" />
              Processing fees daily
            </p>
          </div>
        </div>

        <div className="space-y-2">
          <div className="flex items-center justify-between text-sm">
            <span className="text-muted-foreground">
              Top Package Performance
            </span>
            <span className="text-muted-foreground">
              {analytics.transactionCount} transactions
            </span>
          </div>
          {analytics.packageBreakdown.slice(0, 3).map((pkg, index) => (
            <div key={pkg.packageId}>
              <div className="mb-1 flex items-center justify-between text-sm">
                <span className="truncate font-medium">{pkg.packageName}</span>
                <span className="text-muted-foreground">{pkg.percentage}%</span>
              </div>
              <Progress value={pkg.percentage} className="h-2" />
            </div>
          ))}
        </div>

        <div className="mt-4 border-t pt-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-sm">
              <DollarSign className="h-4 w-4 text-amber-600" />
              <span className="text-muted-foreground">Waived this month:</span>
              <span className="font-medium">
                ${analytics.waivedAmount.toLocaleString()}
              </span>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => router.push('/plugins/fees/calculator')}
            >
              Calculator
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
