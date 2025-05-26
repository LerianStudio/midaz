'use client'

import { useState } from 'react'
import Link from 'next/link'
import { formatDistanceToNow } from 'date-fns'
import {
  MoreHorizontal,
  Eye,
  Edit,
  Copy,
  Download,
  Trash2,
  FileText,
  Activity,
  Calendar,
  Users,
  TrendingUp
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { useToast } from '@/hooks/use-toast'

interface Template {
  id: string
  name: string
  description: string
  category: string
  tags: string[]
  status: 'active' | 'inactive' | 'draft'
  usageCount: number
  lastUsed: string
  createdBy: string
  createdAt: string
  fileSize: number
  version: string
}

interface TemplateCardProps {
  template: Template
  onEdit?: (template: Template) => void
  onDuplicate?: (template: Template) => void
  onDelete?: (template: Template) => void
  onPreview?: (template: Template) => void
  className?: string
}

const categoryColors = {
  financial_reports: 'bg-blue-100 text-blue-800 border-blue-200',
  receipts: 'bg-green-100 text-green-800 border-green-200',
  contracts: 'bg-purple-100 text-purple-800 border-purple-200',
  statements: 'bg-orange-100 text-orange-800 border-orange-200',
  notifications: 'bg-yellow-100 text-yellow-800 border-yellow-200',
  custom: 'bg-gray-100 text-gray-800 border-gray-200'
}

const statusColors = {
  active: 'bg-green-100 text-green-800 border-green-200',
  inactive: 'bg-gray-100 text-gray-800 border-gray-200',
  draft: 'bg-yellow-100 text-yellow-800 border-yellow-200'
}

export function TemplateCard({
  template,
  onEdit,
  onDuplicate,
  onDelete,
  onPreview,
  className
}: TemplateCardProps) {
  const { toast } = useToast()
  const [isHovered, setIsHovered] = useState(false)

  const handleEdit = () => {
    onEdit?.(template)
  }

  const handleDuplicate = () => {
    onDuplicate?.(template)
    toast({
      title: 'Template Duplicated',
      description: `Created a copy of "${template.name}"`
    })
  }

  const handleDelete = () => {
    if (confirm(`Are you sure you want to delete "${template.name}"?`)) {
      onDelete?.(template)
      toast({
        title: 'Template Deleted',
        description: `"${template.name}" has been deleted`,
        variant: 'destructive'
      })
    }
  }

  const handlePreview = () => {
    onPreview?.(template)
  }

  const formatFileSize = (bytes: number) => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
  }

  const getCategoryColor = (category: string) => {
    return (
      categoryColors[category as keyof typeof categoryColors] ||
      categoryColors.custom
    )
  }

  const getStatusColor = (status: string) => {
    return (
      statusColors[status as keyof typeof statusColors] || statusColors.draft
    )
  }

  return (
    <Card
      className={`transition-all duration-200 hover:shadow-md ${isHovered ? 'ring-2 ring-blue-200' : ''} ${className}`}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="min-w-0 flex-1">
            <div className="mb-2 flex items-center gap-2">
              <Badge className={getCategoryColor(template.category)}>
                {template.category.replace('_', ' ')}
              </Badge>
              <Badge className={getStatusColor(template.status)}>
                {template.status}
              </Badge>
            </div>
            <CardTitle className="mb-1 text-lg leading-tight">
              <Link
                href={`/plugins/smart-templates/templates/${template.id}`}
                className="transition-colors hover:text-blue-600"
              >
                {template.name}
              </Link>
            </CardTitle>
            <CardDescription className="text-sm">
              {template.description}
            </CardDescription>
          </div>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link
                  href={`/plugins/smart-templates/templates/${template.id}`}
                >
                  <Eye className="mr-2 h-4 w-4" />
                  View Details
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link
                  href={`/plugins/smart-templates/templates/${template.id}/preview`}
                >
                  <FileText className="mr-2 h-4 w-4" />
                  Preview
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link
                  href={`/plugins/smart-templates/templates/${template.id}/edit`}
                >
                  <Edit className="mr-2 h-4 w-4" />
                  Edit Template
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem onClick={handleDuplicate}>
                <Copy className="mr-2 h-4 w-4" />
                Duplicate
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onClick={handleDelete}
                className="text-red-600 focus:text-red-600"
              >
                <Trash2 className="mr-2 h-4 w-4" />
                Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {/* Tags */}
        {template.tags.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {template.tags.slice(0, 3).map((tag) => (
              <Badge key={tag} variant="outline" className="text-xs">
                {tag}
              </Badge>
            ))}
            {template.tags.length > 3 && (
              <Badge variant="outline" className="text-xs">
                +{template.tags.length - 3} more
              </Badge>
            )}
          </div>
        )}

        {/* Metrics */}
        <div className="grid grid-cols-2 gap-4 text-sm">
          <div className="flex items-center gap-2">
            <Activity className="h-4 w-4 text-gray-400" />
            <div>
              <div className="font-medium">
                {template.usageCount.toLocaleString()}
              </div>
              <div className="text-xs text-gray-500">uses</div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Calendar className="h-4 w-4 text-gray-400" />
            <div>
              <div className="font-medium">
                {formatDistanceToNow(new Date(template.lastUsed), {
                  addSuffix: true
                })}
              </div>
              <div className="text-xs text-gray-500">last used</div>
            </div>
          </div>
        </div>

        {/* File info */}
        <div className="flex items-center justify-between border-t pt-2 text-xs text-gray-500">
          <div className="flex items-center gap-4">
            <span>{formatFileSize(template.fileSize)}</span>
            <span>v{template.version}</span>
          </div>
          <span>by {template.createdBy.split('@')[0]}</span>
        </div>

        {/* Quick Actions */}
        <div className="flex gap-2 pt-2">
          <Button asChild variant="outline" size="sm" className="flex-1">
            <Link
              href={`/plugins/smart-templates/templates/${template.id}/preview`}
            >
              <Eye className="mr-2 h-4 w-4" />
              Preview
            </Link>
          </Button>
          <Button asChild size="sm" className="flex-1">
            <Link
              href={`/plugins/smart-templates/reports/generate?template=${template.id}`}
            >
              <Download className="mr-2 h-4 w-4" />
              Generate
            </Link>
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
