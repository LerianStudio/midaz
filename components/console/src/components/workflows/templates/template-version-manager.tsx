'use client'

import React, { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  GitBranch,
  Clock,
  User,
  Tag,
  Copy,
  Share2,
  Lock,
  Unlock,
  CheckCircle,
  AlertCircle,
  Download,
  Upload,
  History
} from 'lucide-react'
import { WorkflowTemplate } from '@/core/domain/entities/workflow-template'
import { useToast } from '@/hooks/use-toast'

interface TemplateVersionManagerProps {
  template: WorkflowTemplate
  open: boolean
  onOpenChange: (open: boolean) => void
  onVersionCreate?: (version: TemplateVersion) => void
  onShare?: (shareSettings: ShareSettings) => void
}

interface TemplateVersion {
  id: string
  version: string
  description: string
  changes: string[]
  author: string
  createdAt: string
  isPublished: boolean
  isDraft: boolean
  stats?: {
    downloads: number
    usage: number
    rating: number
  }
}

interface ShareSettings {
  visibility: 'private' | 'internal' | 'public'
  allowForking: boolean
  requireAttribution: boolean
  expiresAt?: string
  sharedWith?: string[]
}

// Mock version history
const mockVersions: TemplateVersion[] = [
  {
    id: 'v3',
    version: '3.0.0',
    description: 'Major update with improved error handling',
    changes: [
      'Added comprehensive error handling for all HTTP tasks',
      'Improved retry logic with exponential backoff',
      'New notification task for failures'
    ],
    author: 'john.doe@company.com',
    createdAt: new Date('2025-01-15').toISOString(),
    isPublished: true,
    isDraft: false,
    stats: {
      downloads: 342,
      usage: 1205,
      rating: 4.8
    }
  },
  {
    id: 'v2',
    version: '2.1.0',
    description: 'Added fee calculation step',
    changes: [
      'Integrated fee calculation service',
      'Added conditional logic for fee types',
      'Updated input parameters'
    ],
    author: 'jane.smith@company.com',
    createdAt: new Date('2024-12-20').toISOString(),
    isPublished: true,
    isDraft: false,
    stats: {
      downloads: 567,
      usage: 2341,
      rating: 4.6
    }
  },
  {
    id: 'v1',
    version: '1.0.0',
    description: 'Initial template release',
    changes: ['Basic payment processing workflow', 'Account validation'],
    author: 'admin@company.com',
    createdAt: new Date('2024-11-01').toISOString(),
    isPublished: true,
    isDraft: false,
    stats: {
      downloads: 890,
      usage: 3456,
      rating: 4.2
    }
  }
]

export function TemplateVersionManager({
  template,
  open,
  onOpenChange,
  onVersionCreate,
  onShare
}: TemplateVersionManagerProps) {
  const { toast } = useToast()
  const [activeTab, setActiveTab] = useState('versions')
  const [isCreatingVersion, setIsCreatingVersion] = useState(false)
  const [selectedVersion, setSelectedVersion] = useState<string>(
    mockVersions[0].version
  )

  // Version creation state
  const [newVersion, setNewVersion] = useState({
    version: '',
    description: '',
    changes: '',
    isDraft: false
  })

  // Share settings state
  const [shareSettings, setShareSettings] = useState<ShareSettings>({
    visibility: 'internal',
    allowForking: true,
    requireAttribution: true
  })

  const [sharedEmails, setSharedEmails] = useState('')

  // Create new version
  const handleCreateVersion = async () => {
    if (!newVersion.version || !newVersion.description) {
      toast({
        title: 'Missing information',
        description: 'Please provide version number and description',
        variant: 'destructive'
      })
      return
    }

    setIsCreatingVersion(true)

    // Simulate API call
    await new Promise((resolve) => setTimeout(resolve, 1500))

    const version: TemplateVersion = {
      id: `v${Date.now()}`,
      version: newVersion.version,
      description: newVersion.description,
      changes: newVersion.changes.split('\n').filter((c) => c.trim()),
      author: 'current.user@company.com',
      createdAt: new Date().toISOString(),
      isPublished: !newVersion.isDraft,
      isDraft: newVersion.isDraft
    }

    if (onVersionCreate) {
      onVersionCreate(version)
    }

    toast({
      title: 'Version created',
      description: `Version ${version.version} has been created successfully`
    })

    setIsCreatingVersion(false)
    setNewVersion({ version: '', description: '', changes: '', isDraft: false })
    setActiveTab('versions')
  }

  // Share template
  const handleShare = () => {
    const settings = {
      ...shareSettings,
      sharedWith: sharedEmails
        .split(',')
        .map((email) => email.trim())
        .filter((email) => email)
    }

    if (onShare) {
      onShare(settings)
    }

    toast({
      title: 'Template shared',
      description: `Template has been shared with ${settings.visibility} visibility`
    })
  }

  // Copy share link
  const copyShareLink = () => {
    const shareLink = `${window.location.origin}/templates/${template.id}?version=${selectedVersion}`
    navigator.clipboard.writeText(shareLink)
    toast({
      title: 'Link copied',
      description: 'Template share link has been copied to clipboard'
    })
  }

  // Export version
  const exportVersion = (version: TemplateVersion) => {
    const data = {
      template: {
        ...template,
        version: version.version,
        metadata: {
          ...template.metadata,
          exportedAt: new Date().toISOString(),
          exportedBy: 'current.user@company.com'
        }
      }
    }

    const blob = new Blob([JSON.stringify(data, null, 2)], {
      type: 'application/json'
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${template.name.replace(/\s+/g, '_')}_v${version.version}.json`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)

    toast({
      title: 'Version exported',
      description: `Version ${version.version} has been exported`
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[80vh] max-w-4xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <GitBranch className="h-5 w-5" />
            Template Version Management
          </DialogTitle>
          <DialogDescription>
            Manage versions and sharing settings for &quot;{template.name}&quot;
          </DialogDescription>
        </DialogHeader>

        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList className="grid w-full grid-cols-3">
            <TabsTrigger value="versions">
              <History className="mr-2 h-4 w-4" />
              Version History
            </TabsTrigger>
            <TabsTrigger value="create">
              <Tag className="mr-2 h-4 w-4" />
              Create Version
            </TabsTrigger>
            <TabsTrigger value="share">
              <Share2 className="mr-2 h-4 w-4" />
              Share Settings
            </TabsTrigger>
          </TabsList>

          {/* Version History */}
          <TabsContent value="versions" className="space-y-4">
            <div className="flex items-center justify-between">
              <p className="text-sm text-muted-foreground">
                {mockVersions.length} versions available
              </p>
              <Select
                value={selectedVersion}
                onValueChange={setSelectedVersion}
              >
                <SelectTrigger className="w-[140px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {mockVersions.map((v) => (
                    <SelectItem key={v.id} value={v.version}>
                      v{v.version}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <ScrollArea className="h-[400px] pr-4">
              <div className="space-y-4">
                {mockVersions.map((version, index) => (
                  <Card
                    key={version.id}
                    className={index === 0 ? 'border-primary' : ''}
                  >
                    <CardHeader>
                      <div className="flex items-start justify-between">
                        <div>
                          <CardTitle className="flex items-center gap-2 text-base">
                            v{version.version}
                            {index === 0 && (
                              <Badge variant="default">Latest</Badge>
                            )}
                            {version.isDraft && (
                              <Badge variant="secondary">Draft</Badge>
                            )}
                          </CardTitle>
                          <p className="mt-1 text-sm text-muted-foreground">
                            {version.description}
                          </p>
                        </div>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => exportVersion(version)}
                        >
                          <Download className="h-4 w-4" />
                        </Button>
                      </div>
                    </CardHeader>
                    <CardContent className="space-y-3">
                      {/* Changes */}
                      <div>
                        <p className="mb-1 text-sm font-medium">Changes:</p>
                        <ul className="space-y-1 text-sm text-muted-foreground">
                          {version.changes.map((change, i) => (
                            <li key={i} className="flex items-start gap-2">
                              <CheckCircle className="mt-0.5 h-3 w-3 flex-shrink-0 text-green-500" />
                              {change}
                            </li>
                          ))}
                        </ul>
                      </div>

                      {/* Metadata */}
                      <div className="flex items-center gap-4 text-xs text-muted-foreground">
                        <div className="flex items-center gap-1">
                          <User className="h-3 w-3" />
                          {version.author}
                        </div>
                        <div className="flex items-center gap-1">
                          <Clock className="h-3 w-3" />
                          {new Date(version.createdAt).toLocaleDateString()}
                        </div>
                      </div>

                      {/* Stats */}
                      {version.stats && (
                        <div className="flex items-center gap-4 border-t pt-2">
                          <div className="text-sm">
                            <span className="text-muted-foreground">
                              Downloads:
                            </span>{' '}
                            <span className="font-medium">
                              {version.stats.downloads}
                            </span>
                          </div>
                          <div className="text-sm">
                            <span className="text-muted-foreground">
                              Usage:
                            </span>{' '}
                            <span className="font-medium">
                              {version.stats.usage}
                            </span>
                          </div>
                          <div className="text-sm">
                            <span className="text-muted-foreground">
                              Rating:
                            </span>{' '}
                            <span className="font-medium">
                              ⭐ {version.stats.rating}
                            </span>
                          </div>
                        </div>
                      )}
                    </CardContent>
                  </Card>
                ))}
              </div>
            </ScrollArea>
          </TabsContent>

          {/* Create Version */}
          <TabsContent value="create" className="space-y-4">
            <Alert>
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>
                Creating a new version will save the current template state.
                Make sure all changes are tested before publishing.
              </AlertDescription>
            </Alert>

            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>Version Number</Label>
                  <Input
                    placeholder="e.g., 3.1.0"
                    value={newVersion.version}
                    onChange={(e) =>
                      setNewVersion({ ...newVersion, version: e.target.value })
                    }
                  />
                  <p className="text-xs text-muted-foreground">
                    Follow semantic versioning (MAJOR.MINOR.PATCH)
                  </p>
                </div>
                <div className="space-y-2">
                  <Label>Version Type</Label>
                  <Select
                    onValueChange={(value) => {
                      const current = mockVersions[0].version.split('.')
                      let newVer = ''
                      switch (value) {
                        case 'major':
                          newVer = `${parseInt(current[0]) + 1}.0.0`
                          break
                        case 'minor':
                          newVer = `${current[0]}.${parseInt(current[1]) + 1}.0`
                          break
                        case 'patch':
                          newVer = `${current[0]}.${current[1]}.${parseInt(current[2]) + 1}`
                          break
                      }
                      setNewVersion({ ...newVersion, version: newVer })
                    }}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="Select version type" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="major">
                        Major (Breaking changes)
                      </SelectItem>
                      <SelectItem value="minor">
                        Minor (New features)
                      </SelectItem>
                      <SelectItem value="patch">Patch (Bug fixes)</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className="space-y-2">
                <Label>Version Description</Label>
                <Input
                  placeholder="Brief description of this version"
                  value={newVersion.description}
                  onChange={(e) =>
                    setNewVersion({
                      ...newVersion,
                      description: e.target.value
                    })
                  }
                />
              </div>

              <div className="space-y-2">
                <Label>Change Log</Label>
                <Textarea
                  placeholder="List changes, one per line..."
                  rows={6}
                  value={newVersion.changes}
                  onChange={(e) =>
                    setNewVersion({ ...newVersion, changes: e.target.value })
                  }
                />
              </div>

              <div className="flex items-center space-x-2">
                <Switch
                  id="draft"
                  checked={newVersion.isDraft}
                  onCheckedChange={(checked) =>
                    setNewVersion({ ...newVersion, isDraft: checked })
                  }
                />
                <Label htmlFor="draft" className="text-sm font-normal">
                  Save as draft (won&apos;t be published to users)
                </Label>
              </div>
            </div>
          </TabsContent>

          {/* Share Settings */}
          <TabsContent value="share" className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Visibility Settings</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-3">
                  <Label>Template Visibility</Label>
                  <Select
                    value={shareSettings.visibility}
                    onValueChange={(value: ShareSettings['visibility']) =>
                      setShareSettings({ ...shareSettings, visibility: value })
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="private">
                        <div className="flex items-center gap-2">
                          <Lock className="h-4 w-4" />
                          Private - Only you can access
                        </div>
                      </SelectItem>
                      <SelectItem value="internal">
                        <div className="flex items-center gap-2">
                          <User className="h-4 w-4" />
                          Internal - Your organization only
                        </div>
                      </SelectItem>
                      <SelectItem value="public">
                        <div className="flex items-center gap-2">
                          <Unlock className="h-4 w-4" />
                          Public - Anyone can use
                        </div>
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <div className="flex items-center space-x-2">
                    <Switch
                      id="forking"
                      checked={shareSettings.allowForking}
                      onCheckedChange={(checked) =>
                        setShareSettings({
                          ...shareSettings,
                          allowForking: checked
                        })
                      }
                    />
                    <Label htmlFor="forking" className="text-sm font-normal">
                      Allow others to fork and modify this template
                    </Label>
                  </div>

                  <div className="flex items-center space-x-2">
                    <Switch
                      id="attribution"
                      checked={shareSettings.requireAttribution}
                      onCheckedChange={(checked) =>
                        setShareSettings({
                          ...shareSettings,
                          requireAttribution: checked
                        })
                      }
                    />
                    <Label
                      htmlFor="attribution"
                      className="text-sm font-normal"
                    >
                      Require attribution when using this template
                    </Label>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">
                  Share with Specific Users
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-2">
                  <Label>Email Addresses</Label>
                  <Textarea
                    placeholder="Enter email addresses separated by commas..."
                    rows={3}
                    value={sharedEmails}
                    onChange={(e) => setSharedEmails(e.target.value)}
                  />
                </div>

                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    onClick={copyShareLink}
                    className="flex-1"
                  >
                    <Copy className="mr-2 h-4 w-4" />
                    Copy Share Link
                  </Button>
                  <Button onClick={handleShare} className="flex-1">
                    <Share2 className="mr-2 h-4 w-4" />
                    Update Sharing
                  </Button>
                </div>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Close
          </Button>
          {activeTab === 'create' && (
            <Button
              onClick={handleCreateVersion}
              disabled={
                isCreatingVersion ||
                !newVersion.version ||
                !newVersion.description
              }
            >
              {isCreatingVersion ? 'Creating...' : 'Create Version'}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
