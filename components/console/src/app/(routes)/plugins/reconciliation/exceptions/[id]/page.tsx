'use client'

import { useState, useEffect } from 'react'
import { useParams } from 'next/navigation'
import {
  ArrowLeft,
  AlertTriangle,
  Clock,
  User,
  CheckCircle,
  X,
  FileText,
  Lightbulb,
  MessageSquare,
  Flag,
  Eye,
  ArrowUp,
  ArrowRight,
  Play
} from 'lucide-react'
import Link from 'next/link'

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
import { Separator } from '@/components/ui/separator'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'

import { ReconciliationMockData } from '@/components/reconciliation/mock/reconciliation-mock-data'
import {
  ExceptionEntity,
  ExceptionStatus,
  ExceptionPriority,
  ExceptionReason
} from '@/core/domain/entities/exception-entity'
import { ExternalTransactionEntity } from '@/core/domain/entities/external-transaction-entity'

const getPriorityColor = (priority: ExceptionPriority) => {
  switch (priority) {
    case 'critical':
      return 'bg-red-500'
    case 'high':
      return 'bg-orange-500'
    case 'medium':
      return 'bg-yellow-500'
    case 'low':
      return 'bg-green-500'
    default:
      return 'bg-gray-500'
  }
}

const getStatusColor = (status: ExceptionStatus) => {
  switch (status) {
    case 'resolved':
      return 'bg-green-500'
    case 'investigating':
      return 'bg-blue-500'
    case 'assigned':
      return 'bg-purple-500'
    case 'escalated':
      return 'bg-red-500'
    case 'pending':
      return 'bg-gray-500'
    default:
      return 'bg-gray-500'
  }
}

const getReasonIcon = (reason: ExceptionReason) => {
  switch (reason) {
    case 'no_match_found':
      return <Eye className="h-4 w-4" />
    case 'multiple_matches':
      return <AlertTriangle className="h-4 w-4" />
    case 'low_confidence':
      return <Flag className="h-4 w-4" />
    case 'amount_mismatch':
      return <ArrowRight className="h-4 w-4" />
    case 'date_mismatch':
      return <Clock className="h-4 w-4" />
    default:
      return <AlertTriangle className="h-4 w-4" />
  }
}

export default function ExceptionDetailPage() {
  const params = useParams()
  const exceptionId = params.id as string

  const [exception, setException] = useState<ExceptionEntity | null>(null)
  const [externalTransaction, setExternalTransaction] =
    useState<ExternalTransactionEntity | null>(null)
  const [newNote, setNewNote] = useState('')
  const [selectedAction, setSelectedAction] = useState('')

  useEffect(() => {
    // Simulate data loading
    const exceptions = ReconciliationMockData.generateExceptions(
      'process-1',
      100
    )
    const foundException =
      exceptions.find((e) => e.id === exceptionId) || exceptions[0]

    setException(foundException)

    // Simulate external transaction
    const externalTx = ReconciliationMockData.generateExternalTransactions(
      'import-1',
      1
    )[0]
    setExternalTransaction(externalTx)
  }, [exceptionId])

  const handleAddNote = () => {
    if (!newNote.trim() || !exception) return

    const note = {
      id: crypto.randomUUID(),
      timestamp: new Date().toISOString(),
      author: 'current.user@company.com',
      note: newNote.trim(),
      type: 'investigation' as const
    }

    setException((prev) =>
      prev
        ? {
            ...prev,
            investigationNotes: [...prev.investigationNotes, note]
          }
        : null
    )

    setNewNote('')
  }

  const handleTakeAction = () => {
    if (!selectedAction || !exception) return

    // Simulate action execution
    console.log('Taking action:', selectedAction)

    // Update exception status based on action
    setException((prev) =>
      prev
        ? {
            ...prev,
            status: selectedAction === 'resolve' ? 'resolved' : 'investigating',
            assignedTo: 'current.user@company.com',
            assignedAt: new Date().toISOString()
          }
        : null
    )
  }

  if (!exception || !externalTransaction) {
    return (
      <div className="container mx-auto p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-8 w-1/3 rounded bg-gray-200" />
          <div className="grid grid-cols-2 gap-4">
            <div className="h-64 rounded bg-gray-200" />
            <div className="h-64 rounded bg-gray-200" />
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="container mx-auto space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href="/plugins/reconciliation/exceptions">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-1 h-4 w-4" />
              Back to Exceptions
            </Button>
          </Link>
          <div className="flex items-center gap-2">
            {getReasonIcon(exception.reason)}
            <h1 className="text-2xl font-bold">Exception Resolution</h1>
            <Badge className={getPriorityColor(exception.priority)}>
              {exception.priority.toUpperCase()}
            </Badge>
            <Badge className={getStatusColor(exception.status)}>
              {exception.status.replace('_', ' ').toUpperCase()}
            </Badge>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {exception.status === 'pending' && (
            <Button className="gap-2">
              <User className="h-4 w-4" />
              Assign to Me
            </Button>
          )}

          {exception.escalationLevel === 0 &&
            exception.priority !== 'critical' && (
              <Button variant="outline" className="gap-2">
                <ArrowUp className="h-4 w-4" />
                Escalate
              </Button>
            )}
        </div>
      </div>

      {/* Exception Summary */}
      <Card
        className={
          exception.priority === 'critical'
            ? 'border-red-200 bg-red-50'
            : exception.priority === 'high'
              ? 'border-orange-200 bg-orange-50'
              : exception.priority === 'medium'
                ? 'border-yellow-200 bg-yellow-50'
                : 'border-green-200 bg-green-50'
        }
      >
        <CardContent className="p-6">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
            <div>
              <div className="text-sm font-medium text-muted-foreground">
                Exception Type
              </div>
              <div className="text-lg font-semibold capitalize">
                {exception.reason.replace('_', ' ')}
              </div>
            </div>
            <div>
              <div className="text-sm font-medium text-muted-foreground">
                Category
              </div>
              <div className="text-lg font-semibold capitalize">
                {exception.category}
              </div>
            </div>
            <div>
              <div className="text-sm font-medium text-muted-foreground">
                Created
              </div>
              <div className="text-lg font-semibold">
                {new Date(exception.createdAt).toLocaleDateString()}
              </div>
            </div>
            <div>
              <div className="text-sm font-medium text-muted-foreground">
                Transaction Amount
              </div>
              <div className="text-lg font-semibold">
                ${exception.metadata.transactionAmount.toLocaleString()}
              </div>
            </div>
          </div>

          {exception.assignedTo && (
            <div className="mt-4 border-t pt-4">
              <div className="flex items-center gap-2">
                <User className="h-4 w-4 text-muted-foreground" />
                <span className="text-sm">
                  Assigned to <strong>{exception.assignedTo}</strong>
                  {exception.assignedAt && (
                    <span className="text-muted-foreground">
                      {' '}
                      on {new Date(exception.assignedAt).toLocaleDateString()}
                    </span>
                  )}
                </span>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Transaction Details */}
        <Card>
          <CardHeader>
            <CardTitle>Transaction Details</CardTitle>
            <CardDescription>
              External transaction that caused the exception
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="font-medium">Amount:</span>
                <div className="mt-1 text-lg font-bold">
                  {externalTransaction.currency}{' '}
                  {externalTransaction.amount.toLocaleString()}
                </div>
              </div>
              <div>
                <span className="font-medium">Date:</span>
                <div className="mt-1">
                  {new Date(externalTransaction.date).toLocaleDateString()}
                </div>
              </div>
              <div>
                <span className="font-medium">Reference:</span>
                <div className="mt-1 font-mono text-xs">
                  {externalTransaction.referenceNumber}
                </div>
              </div>
              <div>
                <span className="font-medium">Account:</span>
                <div className="mt-1">{externalTransaction.accountNumber}</div>
              </div>
            </div>

            <Separator />

            <div>
              <span className="font-medium">Description:</span>
              <p className="mt-1 text-sm">{externalTransaction.description}</p>
            </div>

            <Separator />

            <div>
              <span className="font-medium">Source System:</span>
              <div className="mt-1">{externalTransaction.sourceSystem}</div>
            </div>
          </CardContent>
        </Card>

        {/* AI Suggestions */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Lightbulb className="h-5 w-5 text-yellow-500" />
              AI Suggestions
            </CardTitle>
            <CardDescription>
              Recommended actions based on similar cases
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {exception.suggestedActions.map((action, index) => (
                <div key={index} className="rounded-lg border p-3">
                  <div className="mb-2 flex items-start justify-between">
                    <div className="font-medium capitalize">
                      {action.action.replace('_', ' ')}
                    </div>
                    <Badge variant="outline">
                      {(action.confidence * 100).toFixed(0)}% confidence
                    </Badge>
                  </div>

                  <p className="mb-2 text-sm text-muted-foreground">
                    {action.description}
                  </p>

                  <div className="flex flex-wrap gap-1">
                    {action.reasons.map((reason, i) => (
                      <Badge key={i} variant="secondary" className="text-xs">
                        {reason}
                      </Badge>
                    ))}
                  </div>

                  {action.candidateTransactionId && (
                    <div className="mt-2 border-t pt-2">
                      <Link
                        href={`#`}
                        className="text-xs text-blue-600 hover:underline"
                      >
                        View candidate transaction →
                      </Link>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>

      <Tabs defaultValue="resolution" className="w-full">
        <TabsList>
          <TabsTrigger value="resolution">Resolution Workflow</TabsTrigger>
          <TabsTrigger value="investigation">Investigation Notes</TabsTrigger>
          <TabsTrigger value="impact">Impact Analysis</TabsTrigger>
          <TabsTrigger value="timeline">Timeline</TabsTrigger>
        </TabsList>

        <TabsContent value="resolution" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Resolution Actions</CardTitle>
              <CardDescription>
                Select and execute resolution actions for this exception
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div>
                  <label className="text-sm font-medium">Select Action</label>
                  <Select
                    value={selectedAction}
                    onValueChange={setSelectedAction}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="Choose resolution action" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="manual_match">Manual Match</SelectItem>
                      <SelectItem value="investigate">
                        Investigate Further
                      </SelectItem>
                      <SelectItem value="adjustment">
                        Create Adjustment
                      </SelectItem>
                      <SelectItem value="write_off">Write Off</SelectItem>
                      <SelectItem value="escalate">
                        Escalate to Manager
                      </SelectItem>
                      <SelectItem value="resolve">Mark as Resolved</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="flex items-end">
                  <Button
                    onClick={handleTakeAction}
                    disabled={!selectedAction}
                    className="w-full gap-2"
                  >
                    <Play className="h-4 w-4" />
                    Execute Action
                  </Button>
                </div>
              </div>

              {selectedAction && (
                <div className="rounded-lg bg-blue-50 p-4">
                  <h4 className="mb-2 font-medium">
                    Action: {selectedAction.replace('_', ' ')}
                  </h4>
                  <p className="text-sm text-muted-foreground">
                    {selectedAction === 'manual_match' &&
                      'Manually link this transaction with an internal transaction.'}
                    {selectedAction === 'investigate' &&
                      'Add investigation notes and gather more information.'}
                    {selectedAction === 'adjustment' &&
                      'Create a reconciliation adjustment to resolve discrepancies.'}
                    {selectedAction === 'write_off' &&
                      'Write off this transaction as unreconcilable.'}
                    {selectedAction === 'escalate' &&
                      'Escalate this exception to a manager for review.'}
                    {selectedAction === 'resolve' &&
                      'Mark this exception as resolved.'}
                  </p>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="investigation" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Investigation Notes</CardTitle>
              <CardDescription>
                Document your investigation findings and thoughts
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-3">
                <Textarea
                  placeholder="Add investigation note..."
                  value={newNote}
                  onChange={(e) => setNewNote(e.target.value)}
                  className="min-h-[100px]"
                />
                <Button
                  onClick={handleAddNote}
                  disabled={!newNote.trim()}
                  className="gap-2"
                >
                  <MessageSquare className="h-4 w-4" />
                  Add Note
                </Button>
              </div>

              <Separator />

              <div className="space-y-3">
                {exception.investigationNotes.length === 0 ? (
                  <p className="py-8 text-center text-muted-foreground">
                    No investigation notes yet. Add the first note above.
                  </p>
                ) : (
                  exception.investigationNotes.map((note) => (
                    <div key={note.id} className="rounded-lg border p-3">
                      <div className="mb-2 flex items-start justify-between">
                        <div className="text-sm font-medium">{note.author}</div>
                        <div className="text-xs text-muted-foreground">
                          {new Date(note.timestamp).toLocaleString()}
                        </div>
                      </div>
                      <p className="text-sm">{note.note}</p>
                      <Badge variant="outline" className="mt-2 text-xs">
                        {note.type}
                      </Badge>
                    </div>
                  ))
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="impact" className="space-y-4">
          <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Financial Impact</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-3 text-sm">
                  <div className="flex justify-between">
                    <span>Transaction Amount:</span>
                    <span className="font-medium">
                      ${exception.metadata.transactionAmount.toLocaleString()}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span>Source System:</span>
                    <span className="font-medium">
                      {exception.metadata.sourceSystem}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span>Customer Impact:</span>
                    <Badge
                      variant={
                        exception.metadata.customerImpact
                          ? 'destructive'
                          : 'secondary'
                      }
                    >
                      {exception.metadata.customerImpact ? 'Yes' : 'No'}
                    </Badge>
                  </div>
                  <div className="flex justify-between">
                    <span>Regulatory Risk:</span>
                    <Badge
                      variant={
                        exception.metadata.regulatoryRisk
                          ? 'destructive'
                          : 'secondary'
                      }
                    >
                      {exception.metadata.regulatoryRisk ? 'Yes' : 'No'}
                    </Badge>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Risk Assessment</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  <div>
                    <div className="mb-1 flex justify-between text-sm">
                      <span>Financial Risk:</span>
                      <span>
                        {exception.metadata.transactionAmount > 10000
                          ? 'High'
                          : exception.metadata.transactionAmount > 1000
                            ? 'Medium'
                            : 'Low'}
                      </span>
                    </div>
                  </div>

                  <div>
                    <div className="mb-1 flex justify-between text-sm">
                      <span>Compliance Risk:</span>
                      <span>
                        {exception.metadata.regulatoryRisk ? 'High' : 'Low'}
                      </span>
                    </div>
                  </div>

                  <div>
                    <div className="mb-1 flex justify-between text-sm">
                      <span>Customer Impact:</span>
                      <span>
                        {exception.metadata.customerImpact ? 'High' : 'Low'}
                      </span>
                    </div>
                  </div>

                  <div>
                    <div className="mb-1 flex justify-between text-sm">
                      <span>Escalation Level:</span>
                      <span>{exception.escalationLevel}</span>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="timeline" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Exception Timeline</CardTitle>
              <CardDescription>
                Chronological history of this exception
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="flex items-start gap-3 rounded-lg border p-3">
                  <div className="mt-2 h-2 w-2 rounded-full bg-red-500" />
                  <div className="flex-1">
                    <div className="flex items-start justify-between">
                      <div>
                        <div className="font-medium">Exception Created</div>
                        <div className="text-sm text-muted-foreground">
                          {exception.reason.replace('_', ' ')} detected during
                          reconciliation
                        </div>
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {new Date(exception.createdAt).toLocaleString()}
                      </div>
                    </div>
                  </div>
                </div>

                {exception.assignedAt && (
                  <div className="flex items-start gap-3 rounded-lg border p-3">
                    <div className="mt-2 h-2 w-2 rounded-full bg-blue-500" />
                    <div className="flex-1">
                      <div className="flex items-start justify-between">
                        <div>
                          <div className="font-medium">Exception Assigned</div>
                          <div className="text-sm text-muted-foreground">
                            Assigned to {exception.assignedTo}
                          </div>
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {new Date(exception.assignedAt).toLocaleString()}
                        </div>
                      </div>
                    </div>
                  </div>
                )}

                {exception.investigationNotes.map((note) => (
                  <div
                    key={note.id}
                    className="flex items-start gap-3 rounded-lg border p-3"
                  >
                    <div className="mt-2 h-2 w-2 rounded-full bg-yellow-500" />
                    <div className="flex-1">
                      <div className="flex items-start justify-between">
                        <div>
                          <div className="font-medium">
                            Investigation Note Added
                          </div>
                          <div className="text-sm text-muted-foreground">
                            By {note.author}
                          </div>
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {new Date(note.timestamp).toLocaleString()}
                        </div>
                      </div>
                    </div>
                  </div>
                ))}

                {exception.resolvedAt && (
                  <div className="flex items-start gap-3 rounded-lg border p-3">
                    <div className="mt-2 h-2 w-2 rounded-full bg-green-500" />
                    <div className="flex-1">
                      <div className="flex items-start justify-between">
                        <div>
                          <div className="font-medium">Exception Resolved</div>
                          <div className="text-sm text-muted-foreground">
                            Resolved by {exception.resolvedBy}
                          </div>
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {new Date(exception.resolvedAt).toLocaleString()}
                        </div>
                      </div>
                    </div>
                  </div>
                )}

                {exception.status === 'pending' && (
                  <div className="flex items-start gap-3 rounded-lg border border-dashed p-3">
                    <div className="mt-2 h-2 w-2 rounded-full bg-gray-300" />
                    <div className="flex-1">
                      <div className="font-medium text-muted-foreground">
                        Awaiting Action
                      </div>
                      <div className="text-sm text-muted-foreground">
                        This exception is pending resolution
                      </div>
                    </div>
                  </div>
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
