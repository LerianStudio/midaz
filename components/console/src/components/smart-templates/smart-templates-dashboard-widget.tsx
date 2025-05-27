import React from 'react'
import Link from 'next/link'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  FileText,
  Plus,
  TrendingUp,
  Clock,
  Database,
  Download,
  AlertCircle,
  CheckCircle2
} from 'lucide-react'

export function SmartTemplatesDashboardWidget() {
  // Mock data - will be replaced with real data later
  const stats = {
    totalTemplates: 12,
    activeTemplates: 9,
    reportsGenerated: 1547,
    reportsThisMonth: 89,
    dataSources: 3,
    avgGenerationTime: '2.3s'
  }

  const recentTemplates = [
    {
      id: '1',
      name: 'Monthly Account Statement',
      category: 'financial_reports',
      lastUsed: '2 hours ago',
      usageCount: 156,
      status: 'active'
    },
    {
      id: '2',
      name: 'Transaction Receipt',
      category: 'receipts',
      lastUsed: '5 minutes ago',
      usageCount: 2340,
      status: 'active'
    },
    {
      id: '3',
      name: 'KYC Verification Report',
      category: 'compliance',
      lastUsed: '1 day ago',
      usageCount: 67,
      status: 'draft'
    }
  ]

  const recentReports = [
    {
      id: '1',
      templateName: 'Monthly Account Statement',
      format: 'pdf',
      status: 'completed',
      generatedAt: '10 minutes ago',
      downloadCount: 3
    },
    {
      id: '2',
      templateName: 'Transaction Receipt',
      format: 'html',
      status: 'completed',
      generatedAt: '1 hour ago',
      downloadCount: 1
    },
    {
      id: '3',
      templateName: 'Compliance Report',
      format: 'csv',
      status: 'processing',
      generatedAt: '2 hours ago',
      downloadCount: 0
    }
  ]

  return (
    <div className="grid gap-6">
      {/* Stats Overview */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Total Templates
            </CardTitle>
            <FileText className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{stats.totalTemplates}</div>
            <p className="text-xs text-muted-foreground">
              {stats.activeTemplates} active
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Reports Generated
            </CardTitle>
            <TrendingUp className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {stats.reportsGenerated.toLocaleString()}
            </div>
            <p className="text-xs text-muted-foreground">
              +{stats.reportsThisMonth} this month
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Data Sources</CardTitle>
            <Database className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{stats.dataSources}</div>
            <p className="text-xs text-muted-foreground">All connected</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Avg Generation Time
            </CardTitle>
            <Clock className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{stats.avgGenerationTime}</div>
            <p className="text-xs text-muted-foreground">
              -12% from last month
            </p>
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        {/* Recent Templates */}
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle>Recent Templates</CardTitle>
                <CardDescription>Most recently used templates</CardDescription>
              </div>
              <Link href="/plugins/smart-templates/templates">
                <Button size="sm">View All</Button>
              </Link>
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {recentTemplates.map((template) => (
                <div
                  key={template.id}
                  className="flex items-center justify-between"
                >
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-medium">{template.name}</p>
                      <Badge
                        variant={
                          template.status === 'active' ? 'default' : 'secondary'
                        }
                      >
                        {template.status}
                      </Badge>
                    </div>
                    <div className="flex items-center gap-4 text-xs text-muted-foreground">
                      <span>Used {template.lastUsed}</span>
                      <span>{template.usageCount} total uses</span>
                    </div>
                  </div>
                  <Badge variant="outline">{template.category}</Badge>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        {/* Recent Reports */}
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle>Recent Reports</CardTitle>
                <CardDescription>Latest generated reports</CardDescription>
              </div>
              <Link href="/plugins/smart-templates/reports">
                <Button size="sm">View All</Button>
              </Link>
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {recentReports.map((report) => (
                <div
                  key={report.id}
                  className="flex items-center justify-between"
                >
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-medium">
                        {report.templateName}
                      </p>
                      {report.status === 'completed' ? (
                        <CheckCircle2 className="h-4 w-4 text-green-500" />
                      ) : (
                        <AlertCircle className="h-4 w-4 text-yellow-500" />
                      )}
                    </div>
                    <div className="flex items-center gap-4 text-xs text-muted-foreground">
                      <span>Generated {report.generatedAt}</span>
                      <span>{report.downloadCount} downloads</span>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge variant="outline">
                      {report.format.toUpperCase()}
                    </Badge>
                    {report.status === 'completed' && (
                      <Button size="sm" variant="ghost">
                        <Download className="h-4 w-4" />
                      </Button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Quick Actions */}
      <Card>
        <CardHeader>
          <CardTitle>Quick Actions</CardTitle>
          <CardDescription>
            Common template and report operations
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-4">
            <Link href="/plugins/smart-templates/templates/create">
              <Button>
                <Plus className="mr-2 h-4 w-4" />
                Create Template
              </Button>
            </Link>
            <Link href="/plugins/smart-templates/reports/create">
              <Button variant="outline">
                <FileText className="mr-2 h-4 w-4" />
                Generate Report
              </Button>
            </Link>
            <Link href="/plugins/smart-templates/data-sources">
              <Button variant="outline">
                <Database className="mr-2 h-4 w-4" />
                Manage Data Sources
              </Button>
            </Link>
            <Link href="/plugins/smart-templates/analytics">
              <Button variant="outline">
                <TrendingUp className="mr-2 h-4 w-4" />
                View Analytics
              </Button>
            </Link>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
