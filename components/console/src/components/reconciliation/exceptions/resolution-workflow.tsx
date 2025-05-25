'use client'

import { useState } from 'react'
import { CheckCircle, ArrowRight, User, FileText, AlertTriangle, Clock, MessageSquare, Upload, Search, Link, Calculator, X, Play, Pause } from 'lucide-react'

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Progress } from '@/components/ui/progress'

import { ExceptionEntity, ExceptionStatus, ExceptionPriority } from '@/core/domain/entities/exception-entity'

interface WorkflowStep {
  id: string
  title: string
  description: string
  status: 'pending' | 'active' | 'completed' | 'skipped'
  icon: React.ReactNode
  required: boolean
  estimatedTime?: string
}

interface ResolutionAction {
  type: 'manual_match' | 'create_adjustment' | 'investigate' | 'escalate' | 'write_off' | 'mark_resolved'
  label: string
  description: string
  icon: React.ReactNode
  requiresApproval?: boolean
  estimatedTime?: string
}

interface ResolutionWorkflowProps {
  exception: ExceptionEntity
  onActionExecute?: (action: string, data: any) => void
  onStatusUpdate?: (status: ExceptionStatus) => void
  isExecuting?: boolean
}

export function ResolutionWorkflow({ 
  exception, 
  onActionExecute, 
  onStatusUpdate,
  isExecuting = false 
}: ResolutionWorkflowProps) {
  const [activeStep, setActiveStep] = useState(0)
  const [selectedAction, setSelectedAction] = useState<string>('')
  const [actionData, setActionData] = useState<any>({})
  const [workflowProgress, setWorkflowProgress] = useState(0)

  const resolutionActions: ResolutionAction[] = [
    {
      type: 'manual_match',
      label: 'Manual Match',
      description: 'Link this transaction with an internal transaction manually',
      icon: <Link className="h-4 w-4" />,
      estimatedTime: '5-10 min'
    },
    {
      type: 'create_adjustment',
      label: 'Create Adjustment',
      description: 'Create a reconciliation adjustment entry',
      icon: <Calculator className="h-4 w-4" />,
      requiresApproval: true,
      estimatedTime: '10-15 min'
    },
    {
      type: 'investigate',
      label: 'Investigate Further',
      description: 'Gather more information and document findings',
      icon: <Search className="h-4 w-4" />,
      estimatedTime: '15-30 min'
    },
    {
      type: 'escalate',
      label: 'Escalate to Manager',
      description: 'Escalate this exception to management for review',
      icon: <ArrowRight className="h-4 w-4" />,
      estimatedTime: '2-5 min'
    },
    {
      type: 'write_off',
      label: 'Write Off',
      description: 'Mark transaction as unreconcilable write-off',
      icon: <X className="h-4 w-4" />,
      requiresApproval: true,
      estimatedTime: '5-10 min'
    },
    {
      type: 'mark_resolved',
      label: 'Mark Resolved',
      description: 'Mark this exception as resolved',
      icon: <CheckCircle className="h-4 w-4" />,
      estimatedTime: '1-2 min'
    }
  ]

  const workflowSteps: WorkflowStep[] = [
    {
      id: 'analyze',
      title: 'Analyze Exception',
      description: 'Review exception details and understand the issue',
      status: 'completed',
      icon: <Search className="h-4 w-4" />,
      required: true,
      estimatedTime: '2-5 min'
    },
    {
      id: 'investigate',
      title: 'Investigate Root Cause',
      description: 'Research and document the underlying cause',
      status: 'active',
      icon: <MessageSquare className="h-4 w-4" />,
      required: true,
      estimatedTime: '10-20 min'
    },
    {
      id: 'action',
      title: 'Execute Resolution',
      description: 'Take appropriate action to resolve the exception',
      status: 'pending',
      icon: <Play className="h-4 w-4" />,
      required: true,
      estimatedTime: '5-15 min'
    },
    {
      id: 'verify',
      title: 'Verify Resolution',
      description: 'Confirm that the resolution is complete and accurate',
      status: 'pending',
      icon: <CheckCircle className="h-4 w-4" />,
      required: true,
      estimatedTime: '3-5 min'
    },
    {
      id: 'document',
      title: 'Document Results',
      description: 'Record final notes and outcome documentation',
      status: 'pending',
      icon: <FileText className="h-4 w-4" />,
      required: false,
      estimatedTime: '2-5 min'
    }
  ]

  const getStepStatusColor = (status: string) => {
    switch (status) {
      case 'completed': return 'bg-green-500'
      case 'active': return 'bg-blue-500'
      case 'pending': return 'bg-gray-300'
      case 'skipped': return 'bg-yellow-500'
      default: return 'bg-gray-300'
    }
  }

  const handleActionSelect = (actionType: string) => {
    setSelectedAction(actionType)
    setActionData({})
  }

  const executeAction = () => {
    if (!selectedAction) return

    const action = resolutionActions.find(a => a.type === selectedAction)
    if (!action) return

    // Simulate action execution
    onActionExecute?.(selectedAction, actionData)
    
    // Update workflow progress
    setWorkflowProgress(prev => Math.min(100, prev + 25))
    
    // Move to next step
    setActiveStep(prev => Math.min(workflowSteps.length - 1, prev + 1))
  }

  const assignToMe = () => {
    onStatusUpdate?.('assigned')
  }

  const markStepComplete = (stepIndex: number) => {
    setActiveStep(stepIndex + 1)
    setWorkflowProgress(((stepIndex + 1) / workflowSteps.length) * 100)
  }

  return (
    <div className="space-y-6">
      {/* Exception Assignment */}
      {exception.status === 'pending' && (
        <Card className="border-yellow-200 bg-yellow-50">
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <User className="h-5 w-5 text-yellow-600" />
                <div>
                  <div className="font-medium">Exception Unassigned</div>
                  <div className="text-sm text-yellow-700">
                    This exception needs to be assigned before resolution can begin
                  </div>
                </div>
              </div>
              <Button onClick={assignToMe} className="gap-2">
                <User className="h-4 w-4" />
                Assign to Me
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Workflow Progress */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Clock className="h-5 w-5" />
            Resolution Workflow
          </CardTitle>
          <CardDescription>
            Step-by-step process for resolving this exception
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="space-y-2">
            <div className="flex justify-between text-sm">
              <span>Overall Progress</span>
              <span>{workflowProgress.toFixed(0)}%</span>
            </div>
            <Progress value={workflowProgress} className="h-2" />
          </div>

          <div className="space-y-4">
            {workflowSteps.map((step, index) => (
              <div key={step.id} className="flex items-start gap-4">
                <div className={`w-8 h-8 rounded-full flex items-center justify-center ${getStepStatusColor(step.status)}`}>
                  {step.status === 'completed' ? (
                    <CheckCircle className="h-4 w-4 text-white" />
                  ) : (
                    <div className="text-white">{step.icon}</div>
                  )}
                </div>
                
                <div className="flex-1">
                  <div className="flex items-center gap-2 mb-1">
                    <h4 className="font-medium">{step.title}</h4>
                    {step.required && <Badge variant="outline" className="text-xs">Required</Badge>}
                    {step.estimatedTime && (
                      <Badge variant="secondary" className="text-xs">{step.estimatedTime}</Badge>
                    )}
                  </div>
                  <p className="text-sm text-muted-foreground mb-2">{step.description}</p>
                  
                  {step.status === 'active' && (
                    <Button 
                      size="sm" 
                      onClick={() => markStepComplete(index)}
                      className="gap-2"
                    >
                      <CheckCircle className="h-3 w-3" />
                      Complete Step
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Resolution Actions */}
      <Tabs defaultValue="actions" className="w-full">
        <TabsList>
          <TabsTrigger value="actions">Resolution Actions</TabsTrigger>
          <TabsTrigger value="evidence">Evidence Collection</TabsTrigger>
          <TabsTrigger value="approval">Approval Workflow</TabsTrigger>
        </TabsList>

        <TabsContent value="actions" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Select Resolution Action</CardTitle>
              <CardDescription>
                Choose the appropriate action to resolve this exception
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {resolutionActions.map((action) => (
                  <div
                    key={action.type}
                    className={`p-4 border rounded-lg cursor-pointer transition-colors ${
                      selectedAction === action.type 
                        ? 'border-blue-500 bg-blue-50' 
                        : 'border-gray-200 hover:border-gray-300'
                    }`}
                    onClick={() => handleActionSelect(action.type)}
                  >
                    <div className="flex items-start gap-3">
                      <div className="text-blue-500 mt-1">{action.icon}</div>
                      <div className="flex-1">
                        <div className="font-medium mb-1">{action.label}</div>
                        <p className="text-sm text-muted-foreground mb-2">
                          {action.description}
                        </p>
                        <div className="flex gap-2">
                          {action.requiresApproval && (
                            <Badge variant="outline" className="text-xs">
                              Requires Approval
                            </Badge>
                          )}
                          {action.estimatedTime && (
                            <Badge variant="secondary" className="text-xs">
                              {action.estimatedTime}
                            </Badge>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                ))}
              </div>

              {selectedAction && (
                <Card className="border-blue-200 bg-blue-50">
                  <CardContent className="p-4">
                    <h4 className="font-medium mb-3">
                      Configure: {resolutionActions.find(a => a.type === selectedAction)?.label}
                    </h4>
                    
                    {selectedAction === 'manual_match' && (
                      <div className="space-y-3">
                        <div>
                          <label className="text-sm font-medium">Internal Transaction ID</label>
                          <Input 
                            placeholder="Enter transaction ID to match"
                            value={actionData.internalTransactionId || ''}
                            onChange={(e) => setActionData(prev => ({
                              ...prev,
                              internalTransactionId: e.target.value
                            }))}
                          />
                        </div>
                        <div>
                          <label className="text-sm font-medium">Match Confidence</label>
                          <Select 
                            value={actionData.confidence || ''}
                            onValueChange={(value) => setActionData(prev => ({
                              ...prev,
                              confidence: value
                            }))}
                          >
                            <SelectTrigger>
                              <SelectValue placeholder="Select confidence level" />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="high">High Confidence (90%+)</SelectItem>
                              <SelectItem value="medium">Medium Confidence (70-89%)</SelectItem>
                              <SelectItem value="low">Low Confidence (<70%)</SelectItem>
                            </SelectContent>
                          </Select>
                        </div>
                      </div>
                    )}

                    {selectedAction === 'create_adjustment' && (
                      <div className="space-y-3">
                        <div>
                          <label className="text-sm font-medium">Adjustment Amount</label>
                          <Input 
                            type="number"
                            placeholder="0.00"
                            value={actionData.amount || ''}
                            onChange={(e) => setActionData(prev => ({
                              ...prev,
                              amount: e.target.value
                            }))}
                          />
                        </div>
                        <div>
                          <label className="text-sm font-medium">Adjustment Type</label>
                          <Select 
                            value={actionData.adjustmentType || ''}
                            onValueChange={(value) => setActionData(prev => ({
                              ...prev,
                              adjustmentType: value
                            }))}
                          >
                            <SelectTrigger>
                              <SelectValue placeholder="Select adjustment type" />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="timing_difference">Timing Difference</SelectItem>
                              <SelectItem value="amount_variance">Amount Variance</SelectItem>
                              <SelectItem value="fee_adjustment">Fee Adjustment</SelectItem>
                              <SelectItem value="correction">Correction</SelectItem>
                            </SelectContent>
                          </Select>
                        </div>
                      </div>
                    )}

                    {selectedAction === 'investigate' && (
                      <div className="space-y-3">
                        <div>
                          <label className="text-sm font-medium">Investigation Notes</label>
                          <Textarea 
                            placeholder="Document your investigation findings..."
                            value={actionData.notes || ''}
                            onChange={(e) => setActionData(prev => ({
                              ...prev,
                              notes: e.target.value
                            }))}
                            className="min-h-[100px]"
                          />
                        </div>
                        <div>
                          <label className="text-sm font-medium">Next Steps</label>
                          <Select 
                            value={actionData.nextSteps || ''}
                            onValueChange={(value) => setActionData(prev => ({
                              ...prev,
                              nextSteps: value
                            }))}
                          >
                            <SelectTrigger>
                              <SelectValue placeholder="Select next steps" />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="continue_investigation">Continue Investigation</SelectItem>
                              <SelectItem value="escalate">Escalate to Manager</SelectItem>
                              <SelectItem value="external_inquiry">External System Inquiry</SelectItem>
                              <SelectItem value="customer_contact">Contact Customer</SelectItem>
                            </SelectContent>
                          </Select>
                        </div>
                      </div>
                    )}

                    <div className="mt-4 pt-3 border-t">
                      <div className="flex justify-between items-center">
                        <div className="text-sm text-muted-foreground">
                          Ready to execute this action?
                        </div>
                        <Button 
                          onClick={executeAction}
                          disabled={isExecuting}
                          className="gap-2"
                        >
                          {isExecuting ? (
                            <Pause className="h-4 w-4 animate-spin" />
                          ) : (
                            <Play className="h-4 w-4" />
                          )}
                          {isExecuting ? 'Executing...' : 'Execute Action'}
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="evidence" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Evidence Collection</CardTitle>
              <CardDescription>
                Gather and attach supporting documentation for this resolution
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="border-2 border-dashed border-gray-300 rounded-lg p-8 text-center">
                <Upload className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                <h3 className="text-lg font-medium mb-2">Upload Evidence</h3>
                <p className="text-muted-foreground mb-4">
                  Drag and drop files or click to browse for supporting documents
                </p>
                <Button variant="outline" className="gap-2">
                  <Upload className="h-4 w-4" />
                  Choose Files
                </Button>
              </div>

              <div className="space-y-3">
                <h4 className="font-medium">Suggested Evidence Types</h4>
                <div className="grid grid-cols-2 gap-2">
                  <Badge variant="outline" className="justify-center p-2">Bank Statements</Badge>
                  <Badge variant="outline" className="justify-center p-2">Transaction Records</Badge>
                  <Badge variant="outline" className="justify-center p-2">Email Communications</Badge>
                  <Badge variant="outline" className="justify-center p-2">System Screenshots</Badge>
                  <Badge variant="outline" className="justify-center p-2">Calculation Worksheets</Badge>
                  <Badge variant="outline" className="justify-center p-2">External Confirmations</Badge>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="approval" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Approval Workflow</CardTitle>
              <CardDescription>
                Track approval status for actions requiring management authorization
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="p-4 bg-yellow-50 border border-yellow-200 rounded-lg">
                  <div className="flex items-center gap-2 mb-2">
                    <AlertTriangle className="h-5 w-5 text-yellow-600" />
                    <span className="font-medium">Approval Required</span>
                  </div>
                  <p className="text-sm text-yellow-700">
                    This exception requires management approval due to the transaction amount exceeding $10,000.
                  </p>
                </div>

                <div className="space-y-3">
                  <div className="flex items-center gap-3 p-3 border rounded-lg">
                    <div className="w-8 h-8 bg-blue-500 rounded-full flex items-center justify-center">
                      <User className="h-4 w-4 text-white" />
                    </div>
                    <div className="flex-1">
                      <div className="font-medium">Level 1 Approval</div>
                      <div className="text-sm text-muted-foreground">Senior Analyst Review</div>
                    </div>
                    <Badge className="bg-green-500">Pending</Badge>
                  </div>

                  <div className="flex items-center gap-3 p-3 border rounded-lg opacity-60">
                    <div className="w-8 h-8 bg-gray-300 rounded-full flex items-center justify-center">
                      <User className="h-4 w-4 text-white" />
                    </div>
                    <div className="flex-1">
                      <div className="font-medium">Level 2 Approval</div>
                      <div className="text-sm text-muted-foreground">Manager Review</div>
                    </div>
                    <Badge variant="outline">Waiting</Badge>
                  </div>
                </div>

                <Separator />

                <div>
                  <h4 className="font-medium mb-2">Request Approval</h4>
                  <div className="space-y-3">
                    <Textarea 
                      placeholder="Add a message for the approver..."
                      className="min-h-[80px]"
                    />
                    <Button className="gap-2">
                      <User className="h-4 w-4" />
                      Submit for Approval
                    </Button>
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