'use client'

import { useState, useEffect } from 'react'
import {
  AlertTriangle,
  Clock,
  User,
  FileText,
  CheckCircle,
  XCircle,
  Eye,
  MessageSquare,
  Calendar,
  Target,
  Lightbulb,
  Search,
  LinkIcon,
  ArrowRight,
  Save,
  Send,
  Paperclip,
  Users,
  TrendingUp
} from 'lucide-react'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  mockReconciliationExceptions,
  mockExternalTransactions,
  mockInternalTransactions,
  ReconciliationException
} from '@/lib/mock-data/reconciliation-unified'

interface ExceptionResolutionWorkflowProps {
  exceptionId: string
  onResolutionComplete?: (resolution: any) => void
  className?: string
}

interface ResolutionForm {
  resolutionType:
    | 'manual_match'
    | 'adjustment'
    | 'ignore'
    | 'investigate'
    | 'escalate'
  candidateTransactionId?: string
  adjustmentAmount?: number
  adjustmentReason?: string
  comments: string
  assignTo?: string
  dueDate?: string
  priority?: 'low' | 'medium' | 'high' | 'critical'
}

interface InvestigationNote {
  id: string
  timestamp: string
  author: string
  note: string
  attachments?: string[]
  type: 'note' | 'finding' | 'question' | 'resolution'
}

export function ExceptionResolutionWorkflow({
  exceptionId,
  onResolutionComplete,
  className
}: ExceptionResolutionWorkflowProps) {
  const [exception, setException] = useState<ReconciliationException | null>(
    null
  )
  const [resolutionForm, setResolutionForm] = useState<ResolutionForm>({
    resolutionType: 'investigate',
    comments: '',
    priority: 'medium'
  })
  const [investigationNotes, setInvestigationNotes] = useState<
    InvestigationNote[]
  >([])
  const [newNote, setNewNote] = useState('')
  const [candidateTransactions, setCandidateTransactions] = useState<any[]>([])
  const [isSearching, setIsSearching] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')

  useEffect(() => {
    // Load exception data
    const foundException = mockReconciliationExceptions.find(
      (e) => e.id === exceptionId
    )
    if (foundException) {
      setException(foundException)
      setInvestigationNotes(
        foundException.investigationNotes.map((note) => ({
          ...note,
          type: 'note' as const
        }))
      )

      // Set initial form values based on exception
      setResolutionForm((prev) => ({
        ...prev,
        priority: foundException.priority,
        assignTo: foundException.assignedTo
      }))
    }
  }, [exceptionId])

  const searchCandidateTransactions = async (query: string) => {
    setIsSearching(true)

    // Simulate search delay
    await new Promise((resolve) => setTimeout(resolve, 1000))

    // Mock search results - in real implementation, this would query the API
    const candidates = mockInternalTransactions
      .filter(
        (txn) =>
          txn.description.toLowerCase().includes(query.toLowerCase()) ||
          txn.reference.toLowerCase().includes(query.toLowerCase()) ||
          Math.abs(
            parseFloat(txn.amount.toString()) -
              (exception?.externalTransactionId
                ? mockExternalTransactions.find(
                    (e) => e.id === exception.externalTransactionId
                  )?.amount || 0
                : 0)
          ) < 100
      )
      .slice(0, 5)

    setCandidateTransactions(candidates)
    setIsSearching(false)
  }

  const addInvestigationNote = () => {
    if (!newNote.trim()) return

    const note: InvestigationNote = {
      id: `note-${Date.now()}`,
      timestamp: new Date().toISOString(),
      author: 'current-user@company.com',
      note: newNote,
      type: 'note'
    }

    setInvestigationNotes((prev) => [...prev, note])
    setNewNote('')
  }

  const handleResolution = () => {
    const resolution = {
      exceptionId,
      ...resolutionForm,
      resolvedAt: new Date().toISOString(),
      resolvedBy: 'current-user@company.com',
      investigationNotes
    }

    onResolutionComplete?.(resolution)
  }

  const getPriorityColor = (priority: string) => {
    switch (priority) {
      case 'critical':
        return 'text-red-600 bg-red-50 border-red-200'
      case 'high':
        return 'text-orange-600 bg-orange-50 border-orange-200'
      case 'medium':
        return 'text-yellow-600 bg-yellow-50 border-yellow-200'
      case 'low':
        return 'text-gray-600 bg-gray-50 border-gray-200'
      default:
        return 'text-gray-600 bg-gray-50 border-gray-200'
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'pending':
        return 'text-yellow-600 bg-yellow-50 border-yellow-200'
      case 'assigned':
        return 'text-blue-600 bg-blue-50 border-blue-200'
      case 'investigating':
        return 'text-purple-600 bg-purple-50 border-purple-200'
      case 'resolved':
        return 'text-green-600 bg-green-50 border-green-200'
      case 'escalated':
        return 'text-red-600 bg-red-50 border-red-200'
      default:
        return 'text-gray-600 bg-gray-50 border-gray-200'
    }
  }

  const getResolutionTypeInfo = (type: string) => {
    switch (type) {
      case 'manual_match':
        return {
          title: 'Manual Match',
          description: 'Link this exception to a specific internal transaction',
          icon: LinkIcon,
          color: 'text-blue-600'
        }
      case 'adjustment':
        return {
          title: 'Create Adjustment',
          description: 'Create an adjustment entry to reconcile the difference',
          icon: TrendingUp,
          color: 'text-green-600'
        }
      case 'ignore':
        return {
          title: 'Ignore Exception',
          description: 'Mark this exception as acceptable and ignore',
          icon: XCircle,
          color: 'text-gray-600'
        }
      case 'investigate':
        return {
          title: 'Further Investigation',
          description: 'Assign for additional investigation and analysis',
          icon: Search,
          color: 'text-purple-600'
        }
      case 'escalate':
        return {
          title: 'Escalate Issue',
          description: 'Escalate to supervisor or specialist team',
          icon: ArrowRight,
          color: 'text-red-600'
        }
      default:
        return {
          title: 'Unknown',
          description: 'Unknown resolution type',
          icon: Eye,
          color: 'text-gray-600'
        }
    }
  }

  if (!exception) {
    return (
      <Card className={className}>
        <CardContent className="p-8 text-center">
          <AlertTriangle className="mx-auto mb-4 h-16 w-16 text-muted-foreground" />
          <h3 className="mb-2 text-lg font-medium">Exception Not Found</h3>
          <p className="text-muted-foreground">
            The requested exception could not be loaded.
          </p>
        </CardContent>
      </Card>
    )
  }

  const externalTransaction = exception.externalTransactionId
    ? mockExternalTransactions.find(
        (t) => t.id === exception.externalTransactionId
      )
    : null

  const internalTransaction = exception.internalTransactionId
    ? mockInternalTransactions.find(
        (t) => t.id === exception.internalTransactionId
      )
    : null

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Exception Header */}
      <Card>
        <CardHeader>
          <div className="flex items-start justify-between">
            <div className="space-y-2">
              <CardTitle className="flex items-center gap-2">
                <AlertTriangle className="h-5 w-5 text-orange-500" />
                Exception Resolution
              </CardTitle>
              <CardDescription>
                Resolve reconciliation exception through investigation and
                action
              </CardDescription>
              <div className="flex items-center gap-4 text-sm">
                <div className="flex items-center gap-2">
                  <Clock className="h-4 w-4 text-muted-foreground" />
                  <span>
                    Created: {new Date(exception.createdAt).toLocaleString()}
                  </span>
                </div>
                {exception.dueDate && (
                  <div className="flex items-center gap-2">
                    <Calendar className="h-4 w-4 text-muted-foreground" />
                    <span>
                      Due: {new Date(exception.dueDate).toLocaleString()}
                    </span>
                  </div>
                )}
                {exception.assignedTo && (
                  <div className="flex items-center gap-2">
                    <User className="h-4 w-4 text-muted-foreground" />
                    <span>Assigned: {exception.assignedTo}</span>
                  </div>
                )}
              </div>
            </div>
            <div className="flex flex-col gap-2 text-right">
              <Badge
                variant="outline"
                className={getPriorityColor(exception.priority)}
              >
                {exception.priority.toUpperCase()} PRIORITY
              </Badge>
              <Badge
                variant="outline"
                className={getStatusColor(exception.status)}
              >
                {exception.status.replace('_', ' ').toUpperCase()}
              </Badge>
              <span className="text-sm text-muted-foreground">
                Escalation Level: {exception.escalationLevel}
              </span>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {/* Exception Details */}
            <div className="rounded-lg border border-orange-200 bg-orange-50 p-4">
              <h4 className="mb-2 font-semibold text-orange-900">
                Exception Details
              </h4>
              <div className="space-y-2 text-sm">
                <div>
                  <strong>Category:</strong>{' '}
                  {exception.category.replace('_', ' ')}
                </div>
                <div>
                  <strong>Reason:</strong> {exception.reason}
                </div>
                {exception.resolutionDetails && (
                  <div>
                    <strong>Previous Resolution:</strong>{' '}
                    {JSON.stringify(exception.resolutionDetails)}
                  </div>
                )}
              </div>
            </div>

            {/* Transaction Information */}
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              {/* External Transaction */}
              {externalTransaction && (
                <div className="rounded-lg border border-blue-200 bg-blue-50 p-4">
                  <h4 className="mb-3 font-semibold text-blue-900">
                    External Transaction
                  </h4>
                  <div className="space-y-2 text-sm">
                    <div className="flex justify-between">
                      <span>Amount:</span>
                      <span className="font-medium">
                        {externalTransaction.currency}{' '}
                        {externalTransaction.amount}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Date:</span>
                      <span>
                        {new Date(
                          externalTransaction.date
                        ).toLocaleDateString()}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Reference:</span>
                      <span className="font-mono">
                        {externalTransaction.reference}
                      </span>
                    </div>
                    <div>
                      <span>Description:</span>
                      <p className="mt-1 rounded bg-blue-100 p-2 text-xs">
                        {externalTransaction.description}
                      </p>
                    </div>
                  </div>
                </div>
              )}

              {/* Internal Transaction */}
              {internalTransaction && (
                <div className="rounded-lg border border-green-200 bg-green-50 p-4">
                  <h4 className="mb-3 font-semibold text-green-900">
                    Internal Transaction
                  </h4>
                  <div className="space-y-2 text-sm">
                    <div className="flex justify-between">
                      <span>Amount:</span>
                      <span className="font-medium">
                        {internalTransaction.currency}{' '}
                        {internalTransaction.amount}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Date:</span>
                      <span>
                        {new Date(
                          internalTransaction.date
                        ).toLocaleDateString()}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Reference:</span>
                      <span className="font-mono">
                        {internalTransaction.reference}
                      </span>
                    </div>
                    <div>
                      <span>Description:</span>
                      <p className="mt-1 rounded bg-green-100 p-2 text-xs">
                        {internalTransaction.description}
                      </p>
                    </div>
                  </div>
                </div>
              )}
            </div>

            {/* AI Suggested Actions */}
            {exception.suggestedActions &&
              exception.suggestedActions.length > 0 && (
                <div className="rounded-lg border border-purple-200 bg-purple-50 p-4">
                  <h4 className="mb-3 flex items-center gap-2 font-semibold text-purple-900">
                    <Lightbulb className="h-4 w-4" />
                    AI Suggested Actions
                  </h4>
                  <div className="space-y-3">
                    {exception.suggestedActions.map((action, index) => (
                      <div key={index} className="rounded border bg-white p-3">
                        <div className="mb-2 flex items-start justify-between">
                          <span className="font-medium">
                            {action.action.replace('_', ' ').toUpperCase()}
                          </span>
                          <div className="flex items-center gap-2">
                            <Progress
                              value={action.confidence * 100}
                              className="h-2 w-16"
                            />
                            <span className="text-sm text-purple-600">
                              {Math.round(action.confidence * 100)}%
                            </span>
                          </div>
                        </div>
                        <p className="text-sm text-gray-600">
                          {action.description}
                        </p>
                        {action.candidateTransactionId && (
                          <Button
                            size="sm"
                            variant="outline"
                            className="mt-2"
                            onClick={() => {
                              setResolutionForm((prev) => ({
                                ...prev,
                                resolutionType: 'manual_match',
                                candidateTransactionId:
                                  action.candidateTransactionId
                              }))
                            }}
                          >
                            <LinkIcon className="mr-1 h-4 w-4" />
                            Use This Match
                          </Button>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}
          </div>
        </CardContent>
      </Card>

      {/* Resolution Workflow */}
      <Tabs defaultValue="resolution" className="w-full">
        <TabsList className="grid w-full grid-cols-3">
          <TabsTrigger value="resolution">Resolution</TabsTrigger>
          <TabsTrigger value="investigation">Investigation</TabsTrigger>
          <TabsTrigger value="history">History</TabsTrigger>
        </TabsList>

        <TabsContent value="resolution" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Target className="h-5 w-5 text-blue-600" />
                Resolution Action
              </CardTitle>
              <CardDescription>
                Choose how to resolve this exception
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Resolution Type Selection */}
              <div className="space-y-4">
                <Label htmlFor="resolutionType">Resolution Type</Label>
                <Select
                  value={resolutionForm.resolutionType}
                  onValueChange={(value: any) =>
                    setResolutionForm((prev) => ({
                      ...prev,
                      resolutionType: value
                    }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select resolution type" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="manual_match">Manual Match</SelectItem>
                    <SelectItem value="adjustment">
                      Create Adjustment
                    </SelectItem>
                    <SelectItem value="ignore">Ignore Exception</SelectItem>
                    <SelectItem value="investigate">
                      Further Investigation
                    </SelectItem>
                    <SelectItem value="escalate">Escalate Issue</SelectItem>
                  </SelectContent>
                </Select>

                {/* Resolution Type Info */}
                {resolutionForm.resolutionType && (
                  <div className="rounded-lg bg-gray-50 p-4">
                    {(() => {
                      const info = getResolutionTypeInfo(
                        resolutionForm.resolutionType
                      )
                      const IconComponent = info.icon
                      return (
                        <div className="flex items-start gap-3">
                          <IconComponent
                            className={`mt-0.5 h-5 w-5 ${info.color}`}
                          />
                          <div>
                            <h5 className="font-medium">{info.title}</h5>
                            <p className="text-sm text-muted-foreground">
                              {info.description}
                            </p>
                          </div>
                        </div>
                      )
                    })()}
                  </div>
                )}
              </div>

              {/* Manual Match Selection */}
              {resolutionForm.resolutionType === 'manual_match' && (
                <div className="space-y-4">
                  <Label>Find Matching Transaction</Label>
                  <div className="flex gap-2">
                    <Input
                      placeholder="Search by amount, description, reference..."
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                    />
                    <Button
                      onClick={() => searchCandidateTransactions(searchQuery)}
                      disabled={isSearching}
                    >
                      {isSearching ? 'Searching...' : 'Search'}
                    </Button>
                  </div>

                  {candidateTransactions.length > 0 && (
                    <div className="space-y-2">
                      <Label>Candidate Transactions</Label>
                      {candidateTransactions.map((candidate) => (
                        <div
                          key={candidate.id}
                          className={`cursor-pointer rounded-lg border p-3 transition-colors ${
                            resolutionForm.candidateTransactionId ===
                            candidate.id
                              ? 'border-blue-500 bg-blue-50'
                              : 'border-gray-200 hover:border-gray-300'
                          }`}
                          onClick={() =>
                            setResolutionForm((prev) => ({
                              ...prev,
                              candidateTransactionId: candidate.id
                            }))
                          }
                        >
                          <div className="flex items-start justify-between">
                            <div>
                              <div className="font-medium">
                                {candidate.currency} {candidate.amount}
                              </div>
                              <div className="text-sm text-gray-600">
                                {candidate.description}
                              </div>
                              <div className="text-xs text-gray-500">
                                Ref: {candidate.reference}
                              </div>
                            </div>
                            <div className="text-sm text-gray-500">
                              {new Date(candidate.date).toLocaleDateString()}
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}

              {/* Adjustment Details */}
              {resolutionForm.resolutionType === 'adjustment' && (
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <Label htmlFor="adjustmentAmount">
                        Adjustment Amount
                      </Label>
                      <Input
                        id="adjustmentAmount"
                        type="number"
                        step="0.01"
                        value={resolutionForm.adjustmentAmount || ''}
                        onChange={(e) =>
                          setResolutionForm((prev) => ({
                            ...prev,
                            adjustmentAmount:
                              parseFloat(e.target.value) || undefined
                          }))
                        }
                      />
                    </div>
                    <div>
                      <Label htmlFor="adjustmentReason">
                        Adjustment Reason
                      </Label>
                      <Select
                        value={resolutionForm.adjustmentReason || ''}
                        onValueChange={(value) =>
                          setResolutionForm((prev) => ({
                            ...prev,
                            adjustmentReason: value
                          }))
                        }
                      >
                        <SelectTrigger>
                          <SelectValue placeholder="Select reason" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="processing_fee">
                            Processing Fee
                          </SelectItem>
                          <SelectItem value="exchange_rate">
                            Exchange Rate Difference
                          </SelectItem>
                          <SelectItem value="timing_difference">
                            Timing Difference
                          </SelectItem>
                          <SelectItem value="data_correction">
                            Data Correction
                          </SelectItem>
                          <SelectItem value="other">Other</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                </div>
              )}

              {/* Assignment and Priority */}
              {(resolutionForm.resolutionType === 'investigate' ||
                resolutionForm.resolutionType === 'escalate') && (
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <Label htmlFor="assignTo">Assign To</Label>
                    <Select
                      value={resolutionForm.assignTo || ''}
                      onValueChange={(value) =>
                        setResolutionForm((prev) => ({
                          ...prev,
                          assignTo: value
                        }))
                      }
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Select assignee" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="analyst@company.com">
                          Analyst Team
                        </SelectItem>
                        <SelectItem value="senior-analyst@company.com">
                          Senior Analyst
                        </SelectItem>
                        <SelectItem value="supervisor@company.com">
                          Supervisor
                        </SelectItem>
                        <SelectItem value="specialist@company.com">
                          Specialist Team
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div>
                    <Label htmlFor="priority">Priority</Label>
                    <Select
                      value={resolutionForm.priority || 'medium'}
                      onValueChange={(value: any) =>
                        setResolutionForm((prev) => ({
                          ...prev,
                          priority: value
                        }))
                      }
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Select priority" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="low">Low</SelectItem>
                        <SelectItem value="medium">Medium</SelectItem>
                        <SelectItem value="high">High</SelectItem>
                        <SelectItem value="critical">Critical</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              )}

              {/* Comments */}
              <div className="space-y-2">
                <Label htmlFor="comments">Resolution Comments</Label>
                <Textarea
                  id="comments"
                  placeholder="Add detailed comments about this resolution..."
                  value={resolutionForm.comments}
                  onChange={(e) =>
                    setResolutionForm((prev) => ({
                      ...prev,
                      comments: e.target.value
                    }))
                  }
                  rows={4}
                />
              </div>

              {/* Action Buttons */}
              <div className="flex justify-end gap-2">
                <Button variant="outline">
                  <Save className="mr-2 h-4 w-4" />
                  Save Draft
                </Button>
                <Button onClick={handleResolution}>
                  <CheckCircle className="mr-2 h-4 w-4" />
                  Complete Resolution
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="investigation" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Search className="h-5 w-5 text-purple-600" />
                Investigation Notes
              </CardTitle>
              <CardDescription>
                Document investigation findings and collaborate with team
                members
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Add New Note */}
              <div className="space-y-4">
                <Label htmlFor="newNote">Add Investigation Note</Label>
                <Textarea
                  id="newNote"
                  placeholder="Document your findings, questions, or observations..."
                  value={newNote}
                  onChange={(e) => setNewNote(e.target.value)}
                  rows={3}
                />
                <div className="flex justify-between">
                  <Button variant="outline" size="sm">
                    <Paperclip className="mr-2 h-4 w-4" />
                    Attach File
                  </Button>
                  <Button
                    onClick={addInvestigationNote}
                    disabled={!newNote.trim()}
                  >
                    <Send className="mr-2 h-4 w-4" />
                    Add Note
                  </Button>
                </div>
              </div>

              <Separator />

              {/* Investigation Timeline */}
              <div className="space-y-4">
                <h4 className="font-semibold">Investigation Timeline</h4>
                {investigationNotes.length === 0 ? (
                  <div className="py-8 text-center text-muted-foreground">
                    <MessageSquare className="mx-auto mb-4 h-16 w-16 opacity-50" />
                    <p>
                      No investigation notes yet. Add the first note to start
                      documenting your findings.
                    </p>
                  </div>
                ) : (
                  <div className="space-y-4">
                    {investigationNotes.map((note) => (
                      <div
                        key={note.id}
                        className="flex gap-4 rounded-lg bg-gray-50 p-4"
                      >
                        <div className="flex-shrink-0">
                          <div className="flex h-8 w-8 items-center justify-center rounded-full bg-purple-100">
                            <MessageSquare className="h-4 w-4 text-purple-600" />
                          </div>
                        </div>
                        <div className="flex-1">
                          <div className="mb-2 flex items-center justify-between">
                            <span className="text-sm font-medium">
                              {note.author}
                            </span>
                            <span className="text-xs text-muted-foreground">
                              {new Date(note.timestamp).toLocaleString()}
                            </span>
                          </div>
                          <p className="text-sm text-gray-700">{note.note}</p>
                          {note.attachments && note.attachments.length > 0 && (
                            <div className="mt-2 flex gap-2">
                              {note.attachments.map((attachment, index) => (
                                <Badge
                                  key={index}
                                  variant="outline"
                                  className="text-xs"
                                >
                                  <Paperclip className="mr-1 h-3 w-3" />
                                  {attachment}
                                </Badge>
                              ))}
                            </div>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="history" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Clock className="h-5 w-5 text-gray-600" />
                Exception History
              </CardTitle>
              <CardDescription>
                Complete timeline of actions and changes for this exception
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {/* Timeline Events */}
                <div className="space-y-4">
                  <div className="flex gap-4 border-l-4 border-orange-500 bg-orange-50 p-4">
                    <AlertTriangle className="mt-0.5 h-5 w-5 flex-shrink-0 text-orange-500" />
                    <div>
                      <div className="font-medium">Exception Created</div>
                      <div className="text-sm text-gray-600">
                        {exception.reason}
                      </div>
                      <div className="mt-1 text-xs text-gray-500">
                        {new Date(exception.createdAt).toLocaleString()}
                      </div>
                    </div>
                  </div>

                  {exception.assignedTo && (
                    <div className="flex gap-4 border-l-4 border-blue-500 bg-blue-50 p-4">
                      <Users className="mt-0.5 h-5 w-5 flex-shrink-0 text-blue-500" />
                      <div>
                        <div className="font-medium">
                          Assigned for Investigation
                        </div>
                        <div className="text-sm text-gray-600">
                          Assigned to {exception.assignedTo}
                        </div>
                        <div className="mt-1 text-xs text-gray-500">
                          {new Date(exception.updatedAt).toLocaleString()}
                        </div>
                      </div>
                    </div>
                  )}

                  {investigationNotes.map((note) => (
                    <div
                      key={note.id}
                      className="flex gap-4 border-l-4 border-purple-500 bg-purple-50 p-4"
                    >
                      <MessageSquare className="mt-0.5 h-5 w-5 flex-shrink-0 text-purple-500" />
                      <div>
                        <div className="font-medium">
                          Investigation Note Added
                        </div>
                        <div className="text-sm text-gray-600">{note.note}</div>
                        <div className="mt-1 text-xs text-gray-500">
                          {note.author} •{' '}
                          {new Date(note.timestamp).toLocaleString()}
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
