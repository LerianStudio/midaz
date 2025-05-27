'use client'

import { useState, useEffect } from 'react'
import { useParams, useRouter } from 'next/navigation'
import {
  ArrowLeft,
  Edit,
  Database,
  ExternalLink,
  Users,
  Activity,
  Calendar,
  TrendingUp,
  Clock,
  BarChart3,
  FileText,
  History
} from 'lucide-react'
import Link from 'next/link'
import { formatDistanceToNow, format } from 'date-fns'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '@/components/page-header'
import {
  mockAccountTypes,
  mockAnalyticsData,
  AccountType
} from '@/core/domain/mock-data/accounting-mock-data'
import { Skeleton } from '@/components/ui/skeleton'

export default function AccountTypeDetailsPage() {
  const params = useParams()
  const router = useRouter()
  const [accountType, setAccountType] = useState<AccountType | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    // Simulate API call
    const fetchAccountType = async () => {
      setIsLoading(true)

      // Simulate network delay
      await new Promise((resolve) => setTimeout(resolve, 500))

      const foundAccountType = mockAccountTypes.find(
        (type) => type.id === params.id
      )
      setAccountType(foundAccountType || null)
      setIsLoading(false)
    }

    fetchAccountType()
  }, [params.id])

  if (isLoading) {
    return (
      <div className="flex h-full flex-col">
        <PageHeader.Root>
          <div className="flex items-center gap-3">
            <Skeleton className="h-8 w-8" />
            <div className="space-y-2">
              <Skeleton className="h-6 w-48" />
              <Skeleton className="h-4 w-64" />
            </div>
          </div>
          <Skeleton className="h-8 w-20" />
        </PageHeader.Root>
        <div className="flex-1 space-y-6 px-6 pb-6">
          <Skeleton className="h-32" />
          <Skeleton className="h-96" />
        </div>
      </div>
    )
  }

  if (!accountType) {
    return (
      <div className="flex h-full flex-col items-center justify-center">
        <div className="space-y-4 text-center">
          <h2 className="text-2xl font-semibold">Account Type Not Found</h2>
          <p className="text-gray-600">
            The account type you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild>
            <Link href="/plugins/accounting/account-types">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Account Types
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  const usageData = mockAnalyticsData.accountTypeUsage.find(
    (usage) => usage.keyValue === accountType.keyValue
  ) || { usageCount: accountType.usageCount, percentage: 0 }

  const auditEntries = mockAnalyticsData.auditTrail.filter(
    (entry) => entry.resourceId === accountType.id
  )

  return (
    <div className="flex h-full flex-col">
      <PageHeader.Root>
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" asChild>
            <Link href="/plugins/accounting/account-types">
              <ArrowLeft className="h-4 w-4" />
            </Link>
          </Button>
          <div>
            <PageHeader.InfoTitle
              title={accountType.name}
              subtitle={accountType.description}
            />
            <div className="mt-2 flex items-center gap-2">
              <code className="rounded bg-gray-100 px-2 py-1 font-mono text-sm">
                {accountType.keyValue}
              </code>
              <Badge
                variant={
                  accountType.domain === 'ledger' ? 'default' : 'secondary'
                }
                className="gap-1"
              >
                {accountType.domain === 'ledger' ? (
                  <Database className="h-3 w-3" />
                ) : (
                  <ExternalLink className="h-3 w-3" />
                )}
                {accountType.domain === 'ledger'
                  ? 'Ledger Domain'
                  : 'External Domain'}
              </Badge>
              <Badge
                variant={
                  accountType.status === 'active'
                    ? 'default'
                    : accountType.status === 'inactive'
                      ? 'secondary'
                      : accountType.status === 'draft'
                        ? 'outline'
                        : 'destructive'
                }
              >
                {accountType.status}
              </Badge>
            </div>
          </div>
        </div>
        <PageHeader.InfoTooltip subtitle="View and manage account type details, analytics, and audit trail." />
        <div className="flex items-center gap-2">
          <Button size="sm">
            <Edit className="mr-2 h-4 w-4" />
            Edit
          </Button>
        </div>
      </PageHeader.Root>

      <div className="flex-1 px-6 pb-6">
        <Tabs defaultValue="overview" className="space-y-6">
          <TabsList className="grid w-full grid-cols-4">
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="analytics">Analytics</TabsTrigger>
            <TabsTrigger value="accounts">Accounts</TabsTrigger>
            <TabsTrigger value="audit">Audit Trail</TabsTrigger>
          </TabsList>

          <TabsContent value="overview" className="space-y-6">
            {/* Key Metrics */}
            <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">
                    Total Usage
                  </CardTitle>
                  <Activity className="h-4 w-4 text-muted-foreground" />
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">
                    {accountType.usageCount.toLocaleString()}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {usageData.percentage.toFixed(1)}% of total
                  </p>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">
                    Linked Accounts
                  </CardTitle>
                  <Users className="h-4 w-4 text-muted-foreground" />
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">
                    {accountType.linkedAccounts}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    Active accounts
                  </p>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">
                    Last Used
                  </CardTitle>
                  <Clock className="h-4 w-4 text-muted-foreground" />
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">
                    {formatDistanceToNow(new Date(accountType.lastUsed), {
                      addSuffix: true
                    })}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {format(new Date(accountType.lastUsed), 'PPp')}
                  </p>
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">Created</CardTitle>
                  <Calendar className="h-4 w-4 text-muted-foreground" />
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">
                    {formatDistanceToNow(new Date(accountType.createdAt), {
                      addSuffix: true
                    })}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {format(new Date(accountType.createdAt), 'PP')}
                  </p>
                </CardContent>
              </Card>
            </div>

            {/* Account Type Information */}
            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <FileText className="h-5 w-5" />
                    Account Type Details
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div>
                    <div className="text-sm font-medium text-gray-600">
                      Name
                    </div>
                    <div className="text-gray-900">{accountType.name}</div>
                  </div>

                  <div>
                    <div className="text-sm font-medium text-gray-600">
                      Description
                    </div>
                    <div className="text-sm leading-relaxed text-gray-700">
                      {accountType.description}
                    </div>
                  </div>

                  <div>
                    <div className="text-sm font-medium text-gray-600">
                      Key Value
                    </div>
                    <code className="rounded bg-gray-100 px-2 py-1 font-mono text-sm">
                      {accountType.keyValue}
                    </code>
                  </div>

                  <div>
                    <div className="text-sm font-medium text-gray-600">
                      Domain
                    </div>
                    <div className="mt-1">
                      <Badge
                        variant={
                          accountType.domain === 'ledger'
                            ? 'default'
                            : 'secondary'
                        }
                        className="gap-1"
                      >
                        {accountType.domain === 'ledger' ? (
                          <Database className="h-3 w-3" />
                        ) : (
                          <ExternalLink className="h-3 w-3" />
                        )}
                        {accountType.domain === 'ledger'
                          ? 'Ledger Domain'
                          : 'External Domain'}
                      </Badge>
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <TrendingUp className="h-5 w-5" />
                    Usage Statistics
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="space-y-4">
                    <div className="flex items-center justify-between">
                      <span className="text-sm text-gray-600">
                        Transaction Count
                      </span>
                      <span className="font-medium">
                        {accountType.usageCount.toLocaleString()}
                      </span>
                    </div>

                    <div className="flex items-center justify-between">
                      <span className="text-sm text-gray-600">
                        Market Share
                      </span>
                      <span className="font-medium">
                        {usageData.percentage.toFixed(1)}%
                      </span>
                    </div>

                    <div className="flex items-center justify-between">
                      <span className="text-sm text-gray-600">
                        Active Accounts
                      </span>
                      <span className="font-medium">
                        {accountType.linkedAccounts}
                      </span>
                    </div>

                    <div className="h-2 w-full rounded-full bg-gray-200">
                      <div
                        className="h-2 rounded-full bg-blue-600"
                        style={{
                          width: `${Math.min(usageData.percentage, 100)}%`
                        }}
                      ></div>
                    </div>
                  </div>
                </CardContent>
              </Card>
            </div>
          </TabsContent>

          <TabsContent value="analytics">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <BarChart3 className="h-5 w-5" />
                  Account Type Analytics
                </CardTitle>
                <CardDescription>
                  Detailed usage analytics and performance metrics for this
                  account type
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="py-8 text-center text-gray-500">
                  <BarChart3 className="mx-auto mb-4 h-12 w-12 text-gray-300" />
                  <p>Analytics dashboard will be implemented here</p>
                  <p className="text-sm">
                    Including usage trends, performance metrics, and comparative
                    analysis
                  </p>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="accounts">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Users className="h-5 w-5" />
                  Linked Accounts
                </CardTitle>
                <CardDescription>
                  Accounts that use this account type
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="py-8 text-center text-gray-500">
                  <Users className="mx-auto mb-4 h-12 w-12 text-gray-300" />
                  <p>Account list will be implemented here</p>
                  <p className="text-sm">
                    Showing all {accountType.linkedAccounts} accounts using this
                    type
                  </p>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="audit">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <History className="h-5 w-5" />
                  Audit Trail
                </CardTitle>
                <CardDescription>
                  Complete history of changes and access to this account type
                </CardDescription>
              </CardHeader>
              <CardContent>
                {auditEntries.length > 0 ? (
                  <div className="space-y-4">
                    {auditEntries.map((entry) => (
                      <div
                        key={entry.id}
                        className="border-l-2 border-blue-200 py-2 pl-4"
                      >
                        <div className="flex items-center justify-between">
                          <div className="font-medium">
                            {entry.action.replace(/_/g, ' ')}
                          </div>
                          <div className="text-sm text-gray-500">
                            {formatDistanceToNow(new Date(entry.timestamp), {
                              addSuffix: true
                            })}
                          </div>
                        </div>
                        <div className="text-sm text-gray-600">
                          {entry.details}
                        </div>
                        <div className="mt-1 text-xs text-gray-500">
                          by {entry.user} from {entry.ipAddress}
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="py-8 text-center text-gray-500">
                    <History className="mx-auto mb-4 h-12 w-12 text-gray-300" />
                    <p>No audit entries found</p>
                    <p className="text-sm">
                      Changes to this account type will appear here
                    </p>
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}
