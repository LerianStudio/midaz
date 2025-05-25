'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { Progress } from '@/components/ui/progress'
import {
  ArrowLeft,
  Edit,
  Copy,
  Download,
  Trash2,
  Play,
  MoreHorizontal,
  FileText,
  Users,
  Clock,
  Activity,
  Database,
  Settings,
  Eye,
  Calendar,
  TrendingUp,
  AlertCircle
} from 'lucide-react'
import { Template } from '@/core/domain/entities/template'
import { mockTemplates } from '@/lib/mock-data/smart-templates'

const getInitials = (name: string) => {
  return name
    .split(' ')
    .map(word => word.charAt(0))
    .join('')
    .toUpperCase()
    .slice(0, 2)
}

interface TemplateDetailViewProps {
  templateId: string
}

export function TemplateDetailView({ templateId }: TemplateDetailViewProps) {
  const router = useRouter()
  const [template] = useState<Template>(
    mockTemplates.find((t) => t.id === templateId) || mockTemplates[0]
  )

  const statusColors = {
    draft: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
    active: 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200',
    inactive:
      'bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-200',
    archived: 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200'
  } as const

  const formatDate = (date: string) => {
    return new Date(date).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    })
  }

  const handleEdit = () => {
    router.push(`/plugins/smart-templates/templates/${template.id}/edit`)
  }

  const handleDuplicate = () => {
    console.log('Duplicating template:', template.id)
  }

  const handleDelete = () => {
    if (confirm('Are you sure you want to delete this template?')) {
      console.log('Deleting template:', template.id)
      router.push('/plugins/smart-templates/templates')
    }
  }

  const handleGenerateReport = () => {
    router.push(
      `/plugins/smart-templates/reports/generate?templateId=${template.id}`
    )
  }

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="flex items-center space-x-4">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => router.back()}
            className="flex items-center space-x-2"
          >
            <ArrowLeft className="h-4 w-4" />
            <span>Back</span>
          </Button>
          <div>
            <div className="mb-2 flex items-center space-x-3">
              <h1 className="text-2xl font-bold">{template.name}</h1>
              <Badge className={statusColors[template.status]}>
                {template.status}
              </Badge>
            </div>
            <p className="text-muted-foreground">{template.description}</p>
          </div>
        </div>

        <div className="flex items-center space-x-2">
          <Button
            onClick={handleGenerateReport}
            className="flex items-center space-x-2"
          >
            <Play className="h-4 w-4" />
            <span>Generate Report</span>
          </Button>
          <Button
            variant="outline"
            onClick={handleEdit}
            className="flex items-center space-x-2"
          >
            <Edit className="h-4 w-4" />
            <span>Edit</span>
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="icon">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={handleDuplicate}>
                <Copy className="mr-2 h-4 w-4" />
                Duplicate
              </DropdownMenuItem>
              <DropdownMenuItem>
                <Download className="mr-2 h-4 w-4" />
                Export
              </DropdownMenuItem>
              <DropdownMenuItem>
                <Eye className="mr-2 h-4 w-4" />
                Preview
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={handleDelete} className="text-red-600">
                <Trash2 className="mr-2 h-4 w-4" />
                Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Quick Stats */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <FileText className="h-5 w-5 text-blue-500" />
              <div>
                <p className="text-sm text-muted-foreground">
                  Reports Generated
                </p>
                <p className="text-2xl font-bold">1,234</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <TrendingUp className="h-5 w-5 text-green-500" />
              <div>
                <p className="text-sm text-muted-foreground">Success Rate</p>
                <p className="text-2xl font-bold">98.5%</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <Clock className="h-5 w-5 text-orange-500" />
              <div>
                <p className="text-sm text-muted-foreground">
                  Avg. Generation Time
                </p>
                <p className="text-2xl font-bold">2.3s</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center space-x-2">
              <Users className="h-5 w-5 text-purple-500" />
              <div>
                <p className="text-sm text-muted-foreground">Active Users</p>
                <p className="text-2xl font-bold">45</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Main Content */}
      <Tabs defaultValue="overview" className="space-y-6">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="content">Template Content</TabsTrigger>
          <TabsTrigger value="datasources">Data Sources</TabsTrigger>
          <TabsTrigger value="reports">Generated Reports</TabsTrigger>
          <TabsTrigger value="analytics">Analytics</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-6">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
            {/* Template Info */}
            <div className="space-y-6 lg:col-span-2">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center space-x-2">
                    <FileText className="h-5 w-5" />
                    <span>Template Information</span>
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <p className="mb-1 text-sm text-muted-foreground">
                        Category
                      </p>
                      <Badge variant="secondary">{template.category}</Badge>
                    </div>
                    <div>
                      <p className="mb-1 text-sm text-muted-foreground">
                        Format
                      </p>
                      <Badge variant="outline">{template.format || 'Unknown'}</Badge>
                    </div>
                    <div>
                      <p className="mb-1 text-sm text-muted-foreground">
                        Engine
                      </p>
                      <p className="font-medium">{template.engine || 'Default'}</p>
                    </div>
                    <div>
                      <p className="mb-1 text-sm text-muted-foreground">
                        Version
                      </p>
                      <p className="font-medium">v{template.version || '1.0'}</p>
                    </div>
                  </div>
                  <div>
                    <p className="mb-1 text-sm text-muted-foreground">Tags</p>
                    <div className="flex flex-wrap gap-1">
                      {template.tags.map((tag) => (
                        <Badge key={tag} variant="outline" className="text-xs">
                          {tag}
                        </Badge>
                      ))}
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center space-x-2">
                    <Activity className="h-5 w-5" />
                    <span>Recent Activity</span>
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="space-y-4">
                    {[
                      {
                        action: 'Report generated',
                        user: 'John Doe',
                        time: '2 hours ago',
                        success: true
                      },
                      {
                        action: 'Template updated',
                        user: 'Jane Smith',
                        time: '1 day ago',
                        success: true
                      },
                      {
                        action: 'Report generation failed',
                        user: 'Bob Wilson',
                        time: '2 days ago',
                        success: false
                      },
                      {
                        action: 'Template created',
                        user: 'Alice Johnson',
                        time: '1 week ago',
                        success: true
                      }
                    ].map((activity, index) => (
                      <div
                        key={index}
                        className="flex items-center space-x-3 rounded-lg bg-muted/30 p-3"
                      >
                        <div
                          className={`h-2 w-2 rounded-full ${activity.success ? 'bg-green-500' : 'bg-red-500'}`}
                        />
                        <div className="flex-1">
                          <p className="text-sm font-medium">
                            {activity.action}
                          </p>
                          <p className="text-xs text-muted-foreground">
                            by {activity.user} • {activity.time}
                          </p>
                        </div>
                      </div>
                    ))}
                  </div>
                </CardContent>
              </Card>
            </div>

            {/* Sidebar */}
            <div className="space-y-6">
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">Details</CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div>
                    <p className="text-sm text-muted-foreground">Created</p>
                    <p className="text-sm font-medium">
                      {formatDate(template.createdAt)}
                    </p>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">
                      Last Modified
                    </p>
                    <p className="text-sm font-medium">
                      {formatDate(template.updatedAt)}
                    </p>
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Created By</p>
                    <div className="mt-1 flex items-center space-x-2">
                      <Avatar className="h-6 w-6">
                        <AvatarFallback>
                          {getInitials(template.createdBy)}
                        </AvatarFallback>
                      </Avatar>
                      <span className="text-sm font-medium">
                        {template.createdBy}
                      </span>
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center space-x-2 text-base">
                    <Database className="h-4 w-4" />
                    <span>Data Sources</span>
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-2">
                  {(template.dataSourceIds || []).map((dsId: string) => (
                    <div
                      key={dsId}
                      className="flex items-center space-x-2 rounded-md bg-muted/30 p-2"
                    >
                      <div className="h-2 w-2 rounded-full bg-green-500" />
                      <span className="text-sm font-medium">
                        Data Source {dsId.split('-')[1]}
                      </span>
                    </div>
                  ))}
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle className="text-base">Health Status</CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm">Template Validation</span>
                    <Badge className="bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200">
                      Valid
                    </Badge>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm">Data Source Connectivity</span>
                    <Badge className="bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200">
                      Connected
                    </Badge>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm">Performance</span>
                    <Badge className="bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200">
                      Optimal
                    </Badge>
                  </div>
                </CardContent>
              </Card>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="content" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Template Content</CardTitle>
              <CardDescription>
                View and edit your template structure and content
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="rounded-lg bg-muted/30 p-4">
                <pre className="overflow-x-auto text-sm">
                  {template.content ||
                    '# Template Content\n\nYour template content will appear here once you edit the template.'}
                </pre>
              </div>
              <div className="mt-4 flex justify-end">
                <Button onClick={handleEdit}>
                  <Edit className="mr-2 h-4 w-4" />
                  Edit Content
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="datasources" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Connected Data Sources</CardTitle>
              <CardDescription>
                Manage data sources connected to this template
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {(template.dataSourceIds || []).map((dsId: string, index: number) => (
                  <div
                    key={dsId}
                    className="flex items-center justify-between rounded-lg border p-4"
                  >
                    <div className="flex items-center space-x-3">
                      <Database className="h-5 w-5 text-blue-500" />
                      <div>
                        <h4 className="font-medium">Data Source {index + 1}</h4>
                        <p className="text-sm text-muted-foreground">
                          Connected and active
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center space-x-2">
                      <Badge className="bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200">
                        Connected
                      </Badge>
                      <Button variant="outline" size="sm">
                        Configure
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="reports" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Generated Reports</CardTitle>
              <CardDescription>
                View and manage reports generated from this template
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="py-8 text-center text-muted-foreground">
                <FileText className="mx-auto mb-4 h-12 w-12 opacity-50" />
                <p>Recent reports will appear here</p>
                <Button className="mt-4" onClick={handleGenerateReport}>
                  Generate First Report
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="analytics" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Performance Analytics</CardTitle>
              <CardDescription>
                Monitor template usage and performance metrics
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                <div className="space-y-4">
                  <h4 className="font-medium">Generation Success Rate</h4>
                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Success Rate</span>
                      <span>98.5%</span>
                    </div>
                    <Progress value={98.5} className="h-2" />
                  </div>
                </div>
                <div className="space-y-4">
                  <h4 className="font-medium">Average Generation Time</h4>
                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Performance</span>
                      <span>2.3s (Excellent)</span>
                    </div>
                    <Progress value={85} className="h-2" />
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="settings" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center space-x-2">
                <Settings className="h-5 w-5" />
                <span>Template Settings</span>
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-6">
                <div>
                  <h4 className="mb-2 font-medium">General Settings</h4>
                  <div className="space-y-4">
                    <div className="flex items-center justify-between">
                      <div>
                        <p className="font-medium">Auto-generation</p>
                        <p className="text-sm text-muted-foreground">
                          Automatically generate reports on schedule
                        </p>
                      </div>
                      <Button variant="outline" size="sm">
                        Configure
                      </Button>
                    </div>
                    <div className="flex items-center justify-between">
                      <div>
                        <p className="font-medium">Version Control</p>
                        <p className="text-sm text-muted-foreground">
                          Track template changes and versions
                        </p>
                      </div>
                      <Badge className="bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200">
                        Enabled
                      </Badge>
                    </div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
