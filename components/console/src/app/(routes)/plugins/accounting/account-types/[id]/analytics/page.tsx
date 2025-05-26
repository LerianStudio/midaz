'use client'

import React from 'react'
import { useParams } from 'next/navigation'
import { ArrowLeft, TrendingUp, Users, Activity, Calendar } from 'lucide-react'
import Link from 'next/link'
import { formatDistanceToNow } from 'date-fns'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { PageHeader } from '@/components/page-header'
import { mockAccountTypes } from '@/core/domain/mock-data/accounting-mock-data'

// Mock analytics data for the account type
const generateAnalyticsData = (accountTypeId: string) => {
  return {
    usageMetrics: {
      totalTransactions: 1247,
      monthlyGrowth: 15.2,
      avgTransactionValue: 2850.5,
      peakUsageHour: '14:00-15:00'
    },
    accountDistribution: [
      { type: 'Personal', count: 89, percentage: 65 },
      { type: 'Business', count: 34, percentage: 25 },
      { type: 'Corporate', count: 14, percentage: 10 }
    ],
    recentActivity: [
      {
        date: '2025-01-01T14:30:00Z',
        action: 'Account Created',
        details: 'New checking account opened',
        amount: 5000
      },
      {
        date: '2025-01-01T12:15:00Z',
        action: 'Transaction Processed',
        details: 'Payment transaction completed',
        amount: 1250
      },
      {
        date: '2025-01-01T10:45:00Z',
        action: 'Account Updated',
        details: 'Account information modified',
        amount: null
      }
    ],
    complianceMetrics: {
      score: 98.5,
      violations: 2,
      lastAudit: '2024-12-15T00:00:00Z',
      nextAudit: '2025-03-15T00:00:00Z'
    }
  }
}

export default function AccountTypeAnalyticsPage() {
  const params = useParams()
  const accountTypeId = params.id as string

  // Find the account type
  const accountType = mockAccountTypes.find((at) => at.id === accountTypeId)
  const analytics = generateAnalyticsData(accountTypeId)

  if (!accountType) {
    return (
      <div className="flex h-full flex-col items-center justify-center">
        <h1 className="text-2xl font-bold">Account Type Not Found</h1>
        <p className="text-muted-foreground">
          The requested account type could not be found.
        </p>
        <Button asChild className="mt-4">
          <Link href="/plugins/accounting/account-types">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Account Types
          </Link>
        </Button>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <PageHeader.Root>
        <Button variant="outline" size="sm" asChild>
          <Link href={`/plugins/accounting/account-types/${accountTypeId}`}>
            <ArrowLeft className="h-4 w-4" />
          </Link>
        </Button>
        <div>
          <PageHeader.InfoTitle
            title={`${accountType.name} Analytics`}
            subtitle="Detailed usage analytics and performance metrics"
          />
          <div className="mt-2 flex items-center gap-2">
            <code className="rounded bg-gray-100 px-2 py-1 font-mono text-sm">
              {accountType.keyValue}
            </code>
            <span className="text-sm text-muted-foreground">
              Last updated{' '}
              {formatDistanceToNow(new Date(accountType.updatedAt), {
                addSuffix: true
              })}
            </span>
          </div>
        </div>
        <PageHeader.InfoTooltip subtitle="Analytics showing usage patterns, performance metrics, and compliance data for this account type." />
      </PageHeader.Root>

      <div className="flex-1 space-y-6 px-6 pb-6">
        {/* Usage Metrics */}
        <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">
                Total Transactions
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">
                {analytics.usageMetrics.totalTransactions.toLocaleString()}
              </div>
              <div className="flex items-center text-xs text-green-600">
                <TrendingUp className="mr-1 h-3 w-3" />+
                {analytics.usageMetrics.monthlyGrowth}% this month
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">
                Active Accounts
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">
                {accountType.linkedAccounts}
              </div>
              <div className="flex items-center text-xs text-muted-foreground">
                <Users className="mr-1 h-3 w-3" />
                {accountType.usageCount} total usage
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">
                Avg Transaction Value
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">
                ${analytics.usageMetrics.avgTransactionValue.toLocaleString()}
              </div>
              <div className="text-xs text-muted-foreground">
                USD equivalent
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">
                Compliance Score
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold text-green-600">
                {analytics.complianceMetrics.score}%
              </div>
              <div className="text-xs text-muted-foreground">
                {analytics.complianceMetrics.violations} violations
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Account Distribution */}
        <Card>
          <CardHeader>
            <CardTitle>Account Distribution</CardTitle>
            <CardDescription>
              Breakdown of accounts by type for {accountType.name}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {analytics.accountDistribution.map((item, index) => (
                <div key={index} className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className="h-4 w-4 rounded bg-blue-500"
                      style={{
                        backgroundColor: `hsl(${index * 120}, 70%, 50%)`
                      }}
                    />
                    <span className="font-medium">{item.type}</span>
                  </div>
                  <div className="flex items-center gap-4">
                    <span className="text-sm text-muted-foreground">
                      {item.count} accounts
                    </span>
                    <span className="font-medium">{item.percentage}%</span>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        {/* Recent Activity */}
        <Card>
          <CardHeader>
            <CardTitle>Recent Activity</CardTitle>
            <CardDescription>
              Latest transactions and account changes
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {analytics.recentActivity.map((activity, index) => (
                <div
                  key={index}
                  className="flex items-center gap-4 border-b pb-4 last:border-b-0"
                >
                  <div className="rounded-lg bg-blue-50 p-2">
                    <Activity className="h-4 w-4 text-blue-600" />
                  </div>
                  <div className="flex-1">
                    <div className="font-medium">{activity.action}</div>
                    <div className="text-sm text-muted-foreground">
                      {activity.details}
                    </div>
                  </div>
                  <div className="text-right">
                    {activity.amount && (
                      <div className="font-medium">
                        ${activity.amount.toLocaleString()}
                      </div>
                    )}
                    <div className="text-xs text-muted-foreground">
                      {formatDistanceToNow(new Date(activity.date), {
                        addSuffix: true
                      })}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        {/* Compliance Information */}
        <Card>
          <CardHeader>
            <CardTitle>Compliance & Audit</CardTitle>
            <CardDescription>
              Regulatory compliance status and audit information
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
              <div>
                <h4 className="mb-2 font-medium">Compliance Score</h4>
                <div className="mb-1 text-3xl font-bold text-green-600">
                  {analytics.complianceMetrics.score}%
                </div>
                <p className="text-sm text-muted-foreground">
                  {analytics.complianceMetrics.violations} violations found
                </p>
              </div>
              <div>
                <h4 className="mb-2 font-medium">Audit Schedule</h4>
                <div className="space-y-2">
                  <div className="flex items-center gap-2 text-sm">
                    <Calendar className="h-4 w-4" />
                    <span>
                      Last audit:{' '}
                      {formatDistanceToNow(
                        new Date(analytics.complianceMetrics.lastAudit),
                        { addSuffix: true }
                      )}
                    </span>
                  </div>
                  <div className="flex items-center gap-2 text-sm">
                    <Calendar className="h-4 w-4" />
                    <span>
                      Next audit:{' '}
                      {formatDistanceToNow(
                        new Date(analytics.complianceMetrics.nextAudit)
                      )}
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
