'use client'

import React, { useState, useEffect } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  CheckCircle,
  XCircle,
  AlertCircle,
  Play,
  Loader2,
  FileJson,
  Bug,
  Zap
} from 'lucide-react'
import { Workflow, WorkflowTask } from '@/core/domain/entities/workflow'

interface WorkflowValidatorProps {
  workflow: Workflow
  open: boolean
  onOpenChange: (open: boolean) => void
  onTestExecute?: (testData: any) => void
}

interface ValidationIssue {
  type: 'error' | 'warning' | 'info'
  category: 'structure' | 'logic' | 'configuration' | 'performance'
  taskRef?: string
  message: string
  suggestion?: string
}

interface TestResult {
  taskRef: string
  status: 'success' | 'failure' | 'pending'
  duration?: number
  input?: any
  output?: any
  error?: string
}

export function WorkflowValidator({
  workflow,
  open,
  onOpenChange,
  onTestExecute
}: WorkflowValidatorProps) {
  const [activeTab, setActiveTab] = useState('validation')
  const [validationIssues, setValidationIssues] = useState<ValidationIssue[]>(
    []
  )
  const [isValidating, setIsValidating] = useState(false)
  const [testInput, setTestInput] = useState('{}')
  const [testResults, setTestResults] = useState<TestResult[]>([])
  const [isTesting, setIsTesting] = useState(false)
  const [testProgress, setTestProgress] = useState(0)

  // Perform validation when workflow changes or dialog opens
  useEffect(() => {
    if (open) {
      validateWorkflow()
    }
  }, [workflow, open])

  // Validate workflow structure and logic
  const validateWorkflow = async () => {
    setIsValidating(true)
    const issues: ValidationIssue[] = []

    // Simulate async validation
    await new Promise((resolve) => setTimeout(resolve, 500))

    // Basic validation checks
    if (!workflow.name || workflow.name.trim() === '') {
      issues.push({
        type: 'error',
        category: 'structure',
        message: 'Workflow name is required',
        suggestion: 'Provide a descriptive name for your workflow'
      })
    }

    if (!workflow.tasks || workflow.tasks.length === 0) {
      issues.push({
        type: 'error',
        category: 'structure',
        message: 'Workflow must contain at least one task',
        suggestion: 'Add tasks to define your workflow logic'
      })
    }

    // Validate tasks
    const taskRefs = new Set<string>()
    workflow.tasks.forEach((task, index) => {
      // Check for duplicate task references
      if (taskRefs.has(task.taskReferenceName)) {
        issues.push({
          type: 'error',
          category: 'structure',
          taskRef: task.taskReferenceName,
          message: `Duplicate task reference name: ${task.taskReferenceName}`,
          suggestion: 'Each task must have a unique reference name'
        })
      }
      taskRefs.add(task.taskReferenceName)

      // Validate task configuration
      if (!task.name) {
        issues.push({
          type: 'error',
          category: 'configuration',
          taskRef: task.taskReferenceName,
          message: `Task ${index + 1} is missing a name`,
          suggestion: 'Provide a descriptive name for this task'
        })
      }

      // Type-specific validation
      switch (task.type) {
        case 'HTTP':
          if (!task.inputParameters?.http_request?.uri) {
            issues.push({
              type: 'error',
              category: 'configuration',
              taskRef: task.taskReferenceName,
              message: `HTTP task "${task.name}" is missing URI configuration`,
              suggestion: 'Configure the HTTP endpoint URL'
            })
          }
          break

        case 'SWITCH':
          if (!task.inputParameters?.caseValueParam) {
            issues.push({
              type: 'error',
              category: 'configuration',
              taskRef: task.taskReferenceName,
              message: `SWITCH task "${task.name}" is missing case value parameter`,
              suggestion: 'Define the parameter to evaluate for branching'
            })
          }
          if (
            !task.inputParameters?.decisionCases ||
            Object.keys(task.inputParameters.decisionCases || {}).length === 0
          ) {
            issues.push({
              type: 'warning',
              category: 'logic',
              taskRef: task.taskReferenceName,
              message: `SWITCH task "${task.name}" has no decision cases`,
              suggestion: 'Add at least one decision case'
            })
          }
          break

        case 'SUB_WORKFLOW':
          if (!task.inputParameters?.subWorkflowName) {
            issues.push({
              type: 'error',
              category: 'configuration',
              taskRef: task.taskReferenceName,
              message: `SUB_WORKFLOW task "${task.name}" is missing sub-workflow name`,
              suggestion: 'Specify which workflow to execute'
            })
          }
          break
      }

      // Check for unreferenced variables
      const inputStr = JSON.stringify(task.inputParameters || {})
      const variablePattern = /\${[^}]+}/g
      const variables = inputStr.match(variablePattern) || []

      variables.forEach((variable) => {
        const varPath = variable.slice(2, -1) // Remove ${ and }
        if (varPath.startsWith('workflow.input.')) {
          const inputParam = varPath.replace('workflow.input.', '')
          if (!workflow.inputParameters?.includes(inputParam)) {
            issues.push({
              type: 'warning',
              category: 'logic',
              taskRef: task.taskReferenceName,
              message: `Task "${task.name}" references undefined input parameter: ${inputParam}`,
              suggestion: 'Add this parameter to workflow input parameters'
            })
          }
        }
      })
    })

    // Performance warnings
    if (workflow.tasks.length > 50) {
      issues.push({
        type: 'warning',
        category: 'performance',
        message: 'Workflow contains many tasks which may impact performance',
        suggestion: 'Consider breaking down into sub-workflows'
      })
    }

    // Check for potential infinite loops
    const hasPotentialLoop = workflow.tasks.some(
      (task) => task.type === 'FORK_JOIN_DYNAMIC'
    )
    if (hasPotentialLoop) {
      issues.push({
        type: 'info',
        category: 'logic',
        message: 'Workflow contains dynamic fork/join constructs',
        suggestion: 'Ensure fork/join conditions are properly configured'
      })
    }

    setValidationIssues(issues)
    setIsValidating(false)
  }

  // Test workflow execution
  const testWorkflow = async () => {
    setIsTesting(true)
    setTestResults([])
    setTestProgress(0)

    try {
      // Parse test input
      const inputData = JSON.parse(testInput)

      // Initialize test results
      const results: TestResult[] = workflow.tasks.map((task) => ({
        taskRef: task.taskReferenceName,
        status: 'pending'
      }))
      setTestResults(results)

      // Simulate task execution
      for (let i = 0; i < workflow.tasks.length; i++) {
        const task = workflow.tasks[i]
        const startTime = Date.now()

        // Simulate task execution delay
        await new Promise((resolve) =>
          setTimeout(resolve, 500 + Math.random() * 1000)
        )

        const duration = Date.now() - startTime
        const success = Math.random() > 0.2 // 80% success rate for demo

        results[i] = {
          taskRef: task.taskReferenceName,
          status: success ? 'success' : 'failure',
          duration,
          input: { ...inputData, taskIndex: i },
          output: success
            ? {
                result: `Output from ${task.name}`,
                timestamp: new Date().toISOString()
              }
            : undefined,
          error: success ? undefined : 'Simulated task failure for demo'
        }

        setTestResults([...results])
        setTestProgress(((i + 1) / workflow.tasks.length) * 100)
      }

      // Call onTestExecute if provided
      if (onTestExecute) {
        onTestExecute({
          input: inputData,
          results: results
        })
      }
    } catch (error) {
      console.error('Test execution failed:', error)
    } finally {
      setIsTesting(false)
    }
  }

  // Get validation summary
  const getValidationSummary = () => {
    const errors = validationIssues.filter((i) => i.type === 'error').length
    const warnings = validationIssues.filter((i) => i.type === 'warning').length
    const infos = validationIssues.filter((i) => i.type === 'info').length

    return { errors, warnings, infos }
  }

  const { errors, warnings, infos } = getValidationSummary()
  const isValid = errors === 0

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[80vh] max-w-4xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Bug className="h-5 w-5" />
            Workflow Validation & Testing
          </DialogTitle>
          <DialogDescription>
            Validate your workflow structure and test execution with sample data
          </DialogDescription>
        </DialogHeader>

        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="validation" className="flex items-center gap-2">
              <AlertCircle className="h-4 w-4" />
              Validation
              {!isValidating && validationIssues.length > 0 && (
                <Badge variant={errors > 0 ? 'destructive' : 'secondary'}>
                  {validationIssues.length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="testing" className="flex items-center gap-2">
              <Play className="h-4 w-4" />
              Test Execution
            </TabsTrigger>
          </TabsList>

          <TabsContent value="validation" className="space-y-4">
            {/* Validation Summary */}
            <div className="grid grid-cols-3 gap-4">
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="flex items-center gap-2 text-sm font-medium">
                    <XCircle className="h-4 w-4 text-destructive" />
                    Errors
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <p className="text-2xl font-bold">{errors}</p>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="flex items-center gap-2 text-sm font-medium">
                    <AlertCircle className="h-4 w-4 text-yellow-500" />
                    Warnings
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <p className="text-2xl font-bold">{warnings}</p>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="flex items-center gap-2 text-sm font-medium">
                    <AlertCircle className="h-4 w-4 text-blue-500" />
                    Info
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <p className="text-2xl font-bold">{infos}</p>
                </CardContent>
              </Card>
            </div>

            {/* Validation Issues */}
            <ScrollArea className="h-[300px] rounded-md border p-4">
              {isValidating ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-6 w-6 animate-spin" />
                  <span className="ml-2">Validating workflow...</span>
                </div>
              ) : validationIssues.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-8 text-center">
                  <CheckCircle className="mb-2 h-12 w-12 text-green-500" />
                  <p className="text-lg font-medium">Workflow is valid!</p>
                  <p className="text-sm text-muted-foreground">
                    No issues found in your workflow configuration
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {validationIssues.map((issue, index) => (
                    <Alert
                      key={index}
                      variant={
                        issue.type === 'error' ? 'destructive' : 'default'
                      }
                    >
                      <AlertCircle className="h-4 w-4" />
                      <AlertTitle className="flex items-center gap-2">
                        {issue.message}
                        {issue.taskRef && (
                          <Badge variant="outline" className="text-xs">
                            {issue.taskRef}
                          </Badge>
                        )}
                      </AlertTitle>
                      {issue.suggestion && (
                        <AlertDescription className="mt-1">
                          💡 {issue.suggestion}
                        </AlertDescription>
                      )}
                    </Alert>
                  ))}
                </div>
              )}
            </ScrollArea>
          </TabsContent>

          <TabsContent value="testing" className="space-y-4">
            {/* Test Input */}
            <div className="space-y-2">
              <Label>Test Input Data (JSON)</Label>
              <Textarea
                placeholder='{"amount": 1000, "currency": "USD"}'
                className="min-h-[100px] font-mono text-sm"
                value={testInput}
                onChange={(e) => setTestInput(e.target.value)}
                disabled={isTesting}
              />
            </div>

            {/* Test Progress */}
            {isTesting && (
              <div className="space-y-2">
                <div className="flex items-center justify-between text-sm">
                  <span>Testing in progress...</span>
                  <span>{Math.round(testProgress)}%</span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-gray-200">
                  <div
                    className="h-full bg-blue-500 transition-all duration-300"
                    style={{ width: `${testProgress}%` }}
                  />
                </div>
              </div>
            )}

            {/* Test Results */}
            <ScrollArea className="h-[250px] rounded-md border p-4">
              {testResults.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-8 text-center">
                  <Zap className="mb-2 h-12 w-12 text-muted-foreground" />
                  <p className="text-lg font-medium">Ready to test</p>
                  <p className="text-sm text-muted-foreground">
                    Configure test input and run the workflow
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {testResults.map((result, index) => {
                    const task = workflow.tasks.find(
                      (t) => t.taskReferenceName === result.taskRef
                    )
                    return (
                      <Card key={index}>
                        <CardHeader className="pb-2">
                          <CardTitle className="flex items-center justify-between text-sm">
                            <span>{task?.name || result.taskRef}</span>
                            <div className="flex items-center gap-2">
                              {result.duration && (
                                <span className="text-xs text-muted-foreground">
                                  {result.duration}ms
                                </span>
                              )}
                              <Badge
                                variant={
                                  result.status === 'success'
                                    ? 'default'
                                    : result.status === 'failure'
                                      ? 'destructive'
                                      : 'secondary'
                                }
                              >
                                {result.status}
                              </Badge>
                            </div>
                          </CardTitle>
                        </CardHeader>
                        {(result.output || result.error) && (
                          <CardContent>
                            {result.error ? (
                              <p className="text-sm text-destructive">
                                {result.error}
                              </p>
                            ) : (
                              <pre className="overflow-auto rounded bg-muted p-2 text-xs">
                                {JSON.stringify(result.output, null, 2)}
                              </pre>
                            )}
                          </CardContent>
                        )}
                      </Card>
                    )
                  })}
                </div>
              )}
            </ScrollArea>
          </TabsContent>
        </Tabs>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Close
          </Button>
          {activeTab === 'validation' ? (
            <Button onClick={validateWorkflow} disabled={isValidating}>
              {isValidating ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Validating...
                </>
              ) : (
                'Re-validate'
              )}
            </Button>
          ) : (
            <Button onClick={testWorkflow} disabled={isTesting || !isValid}>
              {isTesting ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Testing...
                </>
              ) : (
                <>
                  <Play className="mr-2 h-4 w-4" />
                  Run Test
                </>
              )}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
