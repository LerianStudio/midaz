'use client'

import { useState } from 'react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  CheckCircle,
  XCircle,
  AlertTriangle,
  FileText,
  User,
  Clock,
  Eye,
  ChevronDown,
  ChevronRight
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'

interface AuditEvent {
  id: string
  timestamp: string
  event: string
  entityType: string
  entityId: string
  entityName: string
  userId: string
  userName: string
  userEmail: string
  action: string
  description: string
  metadata: Record<string, any>
  ipAddress: string
  userAgent: string
  status: 'success' | 'failed' | 'warning'
  changes: {
    before: any
    after: any
  }
}

interface AuditTrailTableProps {
  events: AuditEvent[]
}

export function AuditTrailTable({ events }: AuditTrailTableProps) {
  const [selectedEvent, setSelectedEvent] = useState<AuditEvent | null>(null)
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set())

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'success':
        return <CheckCircle className="h-4 w-4 text-green-500" />
      case 'failed':
        return <XCircle className="h-4 w-4 text-red-500" />
      case 'warning':
        return <AlertTriangle className="h-4 w-4 text-yellow-500" />
      default:
        return <FileText className="h-4 w-4 text-gray-500" />
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'success':
        return 'default'
      case 'failed':
        return 'destructive'
      case 'warning':
        return 'secondary'
      default:
        return 'outline'
    }
  }

  const getActionColor = (action: string) => {
    switch (action) {
      case 'CREATE':
        return 'default'
      case 'UPDATE':
        return 'secondary'
      case 'DELETE':
        return 'destructive'
      case 'CREATE_FAILED':
        return 'destructive'
      case 'SYSTEM_CHECK':
        return 'outline'
      default:
        return 'outline'
    }
  }

  const toggleRowExpansion = (eventId: string) => {
    setExpandedRows((prev) => {
      const newSet = new Set(prev)
      if (newSet.has(eventId)) {
        newSet.delete(eventId)
      } else {
        newSet.add(eventId)
      }
      return newSet
    })
  }

  const formatMetadata = (metadata: Record<string, any>) => {
    return Object.entries(metadata).map(([key, value]) => (
      <div key={key} className="flex justify-between">
        <span className="font-medium">{key}:</span>
        <span className="text-muted-foreground">
          {typeof value === 'object' ? JSON.stringify(value) : String(value)}
        </span>
      </div>
    ))
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FileText className="h-5 w-5" />
          Audit Events
        </CardTitle>
        <CardDescription>
          Detailed audit trail of all system activities ({events.length} events)
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[40px]"></TableHead>
                <TableHead>Timestamp</TableHead>
                <TableHead>Event</TableHead>
                <TableHead>Entity</TableHead>
                <TableHead>User</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-[100px]">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {events.map((event) => (
                <>
                  <TableRow key={event.id} className="hover:bg-muted/50">
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => toggleRowExpansion(event.id)}
                      >
                        {expandedRows.has(event.id) ? (
                          <ChevronDown className="h-4 w-4" />
                        ) : (
                          <ChevronRight className="h-4 w-4" />
                        )}
                      </Button>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Clock className="h-4 w-4 text-muted-foreground" />
                        <div>
                          <div className="font-medium">
                            {new Date(event.timestamp).toLocaleDateString()}
                          </div>
                          <div className="text-sm text-muted-foreground">
                            {new Date(event.timestamp).toLocaleTimeString()}
                          </div>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div>
                        <div className="font-medium">{event.event}</div>
                        <div className="max-w-[200px] truncate text-sm text-muted-foreground">
                          {event.description}
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div>
                        <div className="font-medium">{event.entityName}</div>
                        <div className="text-sm text-muted-foreground">
                          {event.entityType} • {event.entityId}
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <User className="h-4 w-4 text-muted-foreground" />
                        <div>
                          <div className="font-medium">{event.userName}</div>
                          <div className="text-sm text-muted-foreground">
                            {event.userEmail}
                          </div>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant={getActionColor(event.action)}>
                        {event.action}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        {getStatusIcon(event.status)}
                        <Badge variant={getStatusColor(event.status)}>
                          {event.status}
                        </Badge>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Dialog>
                        <DialogTrigger asChild>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setSelectedEvent(event)}
                          >
                            <Eye className="h-4 w-4" />
                          </Button>
                        </DialogTrigger>
                        <DialogContent className="max-h-[80vh] max-w-4xl">
                          <DialogHeader>
                            <DialogTitle>Audit Event Details</DialogTitle>
                            <DialogDescription>
                              Complete details for audit event {event.id}
                            </DialogDescription>
                          </DialogHeader>
                          <ScrollArea className="max-h-[60vh]">
                            <div className="space-y-6">
                              {/* Basic Information */}
                              <div className="grid gap-4 md:grid-cols-2">
                                <div className="space-y-2">
                                  <h4 className="font-medium">
                                    Event Information
                                  </h4>
                                  <div className="space-y-1 text-sm">
                                    <div className="flex justify-between">
                                      <span className="font-medium">
                                        Event ID:
                                      </span>
                                      <span className="text-muted-foreground">
                                        {event.id}
                                      </span>
                                    </div>
                                    <div className="flex justify-between">
                                      <span className="font-medium">
                                        Event Type:
                                      </span>
                                      <span className="text-muted-foreground">
                                        {event.event}
                                      </span>
                                    </div>
                                    <div className="flex justify-between">
                                      <span className="font-medium">
                                        Timestamp:
                                      </span>
                                      <span className="text-muted-foreground">
                                        {new Date(
                                          event.timestamp
                                        ).toLocaleString()}
                                      </span>
                                    </div>
                                    <div className="flex justify-between">
                                      <span className="font-medium">
                                        Status:
                                      </span>
                                      <Badge
                                        variant={getStatusColor(event.status)}
                                      >
                                        {event.status}
                                      </Badge>
                                    </div>
                                  </div>
                                </div>

                                <div className="space-y-2">
                                  <h4 className="font-medium">
                                    User Information
                                  </h4>
                                  <div className="space-y-1 text-sm">
                                    <div className="flex justify-between">
                                      <span className="font-medium">User:</span>
                                      <span className="text-muted-foreground">
                                        {event.userName}
                                      </span>
                                    </div>
                                    <div className="flex justify-between">
                                      <span className="font-medium">
                                        Email:
                                      </span>
                                      <span className="text-muted-foreground">
                                        {event.userEmail}
                                      </span>
                                    </div>
                                    <div className="flex justify-between">
                                      <span className="font-medium">
                                        IP Address:
                                      </span>
                                      <span className="text-muted-foreground">
                                        {event.ipAddress}
                                      </span>
                                    </div>
                                    <div className="flex justify-between">
                                      <span className="font-medium">
                                        User Agent:
                                      </span>
                                      <span className="max-w-[150px] truncate text-muted-foreground">
                                        {event.userAgent.split(' ')[0]}
                                      </span>
                                    </div>
                                  </div>
                                </div>
                              </div>

                              {/* Entity Information */}
                              <div>
                                <h4 className="mb-2 font-medium">
                                  Entity Information
                                </h4>
                                <div className="space-y-1 text-sm">
                                  <div className="flex justify-between">
                                    <span className="font-medium">
                                      Entity Type:
                                    </span>
                                    <span className="text-muted-foreground">
                                      {event.entityType}
                                    </span>
                                  </div>
                                  <div className="flex justify-between">
                                    <span className="font-medium">
                                      Entity ID:
                                    </span>
                                    <span className="text-muted-foreground">
                                      {event.entityId}
                                    </span>
                                  </div>
                                  <div className="flex justify-between">
                                    <span className="font-medium">
                                      Entity Name:
                                    </span>
                                    <span className="text-muted-foreground">
                                      {event.entityName}
                                    </span>
                                  </div>
                                  <div className="flex justify-between">
                                    <span className="font-medium">Action:</span>
                                    <Badge
                                      variant={getActionColor(event.action)}
                                    >
                                      {event.action}
                                    </Badge>
                                  </div>
                                </div>
                              </div>

                              {/* Description */}
                              <div>
                                <h4 className="mb-2 font-medium">
                                  Description
                                </h4>
                                <p className="text-sm text-muted-foreground">
                                  {event.description}
                                </p>
                              </div>

                              {/* Metadata */}
                              {Object.keys(event.metadata).length > 0 && (
                                <div>
                                  <h4 className="mb-2 font-medium">Metadata</h4>
                                  <div className="space-y-1 text-sm">
                                    {formatMetadata(event.metadata)}
                                  </div>
                                </div>
                              )}

                              {/* Changes */}
                              {(event.changes.before ||
                                event.changes.after) && (
                                <div>
                                  <h4 className="mb-2 font-medium">Changes</h4>
                                  <div className="grid gap-4 md:grid-cols-2">
                                    <div>
                                      <h5 className="mb-1 text-sm font-medium">
                                        Before
                                      </h5>
                                      <pre className="max-h-[150px] overflow-auto rounded bg-muted p-2 text-xs">
                                        {event.changes.before
                                          ? JSON.stringify(
                                              event.changes.before,
                                              null,
                                              2
                                            )
                                          : 'N/A'}
                                      </pre>
                                    </div>
                                    <div>
                                      <h5 className="mb-1 text-sm font-medium">
                                        After
                                      </h5>
                                      <pre className="max-h-[150px] overflow-auto rounded bg-muted p-2 text-xs">
                                        {event.changes.after
                                          ? JSON.stringify(
                                              event.changes.after,
                                              null,
                                              2
                                            )
                                          : 'N/A'}
                                      </pre>
                                    </div>
                                  </div>
                                </div>
                              )}
                            </div>
                          </ScrollArea>
                        </DialogContent>
                      </Dialog>
                    </TableCell>
                  </TableRow>

                  {/* Expanded row content */}
                  {expandedRows.has(event.id) && (
                    <TableRow>
                      <TableCell colSpan={8} className="bg-muted/30 p-4">
                        <div className="space-y-3">
                          <div className="grid gap-4 md:grid-cols-3">
                            <div>
                              <h5 className="mb-1 text-sm font-medium">
                                Full Description
                              </h5>
                              <p className="text-sm text-muted-foreground">
                                {event.description}
                              </p>
                            </div>
                            <div>
                              <h5 className="mb-1 text-sm font-medium">
                                IP Address
                              </h5>
                              <p className="text-sm text-muted-foreground">
                                {event.ipAddress}
                              </p>
                            </div>
                            <div>
                              <h5 className="mb-1 text-sm font-medium">
                                User Agent
                              </h5>
                              <p className="truncate text-sm text-muted-foreground">
                                {event.userAgent}
                              </p>
                            </div>
                          </div>

                          {Object.keys(event.metadata).length > 0 && (
                            <div>
                              <h5 className="mb-2 text-sm font-medium">
                                Metadata
                              </h5>
                              <div className="grid gap-2 text-sm md:grid-cols-2">
                                {formatMetadata(event.metadata)}
                              </div>
                            </div>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  )}
                </>
              ))}
            </TableBody>
          </Table>

          {events.length === 0 && (
            <div className="py-8 text-center">
              <FileText className="mx-auto h-12 w-12 text-muted-foreground/50" />
              <p className="mt-2 text-muted-foreground">
                No audit events found
              </p>
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
