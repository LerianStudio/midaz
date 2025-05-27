'use client'

import React, { useState, useRef } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Textarea } from '@/components/ui/textarea'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  Download,
  Upload,
  FileJson,
  FileCode,
  Copy,
  Check,
  AlertCircle
} from 'lucide-react'
import { Workflow } from '@/core/domain/entities/workflow'
import { useToast } from '@/hooks/use-toast'

interface WorkflowImportExportProps {
  workflow?: Workflow
  open: boolean
  onOpenChange: (open: boolean) => void
  onImport?: (workflow: Workflow) => void
  mode: 'import' | 'export'
}

type ExportFormat = 'json' | 'yaml' | 'conductor'

export function WorkflowImportExport({
  workflow,
  open,
  onOpenChange,
  onImport,
  mode
}: WorkflowImportExportProps) {
  const { toast } = useToast()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [importContent, setImportContent] = useState('')
  const [exportFormat, setExportFormat] = useState<ExportFormat>('json')
  const [copied, setCopied] = useState(false)
  const [validationError, setValidationError] = useState<string | null>(null)
  const [isValidating, setIsValidating] = useState(false)

  // Convert workflow to different formats
  const convertWorkflowToFormat = (
    workflow: Workflow,
    format: ExportFormat
  ): string => {
    switch (format) {
      case 'json':
        return JSON.stringify(workflow, null, 2)

      case 'yaml':
        // Simplified YAML conversion (in production, use a proper YAML library)
        return convertToYaml(workflow)

      case 'conductor':
        // Convert to Netflix Conductor format
        return JSON.stringify(convertToConductorFormat(workflow), null, 2)

      default:
        return JSON.stringify(workflow, null, 2)
    }
  }

  // Convert to YAML format (simplified)
  const convertToYaml = (workflow: Workflow): string => {
    let yaml = `name: ${workflow.name}\n`
    yaml += `description: ${workflow.description || ''}\n`
    yaml += `version: ${workflow.version}\n`
    yaml += `status: ${workflow.status}\n`
    yaml += `schemaVersion: ${(workflow as any).schemaVersion || 2}\n`
    yaml += `tasks:\n`

    workflow.tasks.forEach((task) => {
      yaml += `  - name: ${task.name}\n`
      yaml += `    taskReferenceName: ${task.taskReferenceName}\n`
      yaml += `    type: ${task.type}\n`
      if (task.inputParameters) {
        yaml += `    inputParameters:\n`
        Object.entries(task.inputParameters).forEach(([key, value]) => {
          yaml += `      ${key}: ${JSON.stringify(value)}\n`
        })
      }
    })

    return yaml
  }

  // Convert to Netflix Conductor format
  const convertToConductorFormat = (workflow: Workflow) => {
    return {
      name: workflow.name,
      description: workflow.description,
      version: workflow.version,
      tasks: workflow.tasks,
      inputParameters: workflow.inputParameters || [],
      outputParameters: workflow.outputParameters || [],
      schemaVersion: (workflow as any).schemaVersion || 2,
      restartable: true,
      workflowStatusListenerEnabled: false,
      ownerEmail:
        workflow.metadata?.ownerEmail ||
        workflow.createdBy ||
        'admin@company.com',
      timeoutSeconds: 0,
      timeoutPolicy: 'TIME_OUT_WF',
      failureWorkflow: '',
      variables: {}
    }
  }

  // Validate imported workflow
  const validateWorkflow = (content: string): Workflow | null => {
    try {
      const parsed = JSON.parse(content)

      // Check required fields
      if (!parsed.name || !parsed.tasks || !Array.isArray(parsed.tasks)) {
        throw new Error('Invalid workflow format: missing required fields')
      }

      // Validate tasks
      for (const task of parsed.tasks) {
        if (!task.name || !task.type || !task.taskReferenceName) {
          throw new Error('Invalid task format: missing required task fields')
        }
      }

      // Set defaults for missing fields
      const workflow: Workflow = {
        id: parsed.id || `imported_${Date.now()}`,
        name: parsed.name,
        description: parsed.description || '',
        version: parsed.version || 1,
        status: parsed.status || 'DRAFT',
        tasks: parsed.tasks,
        inputParameters: parsed.inputParameters || [],
        outputParameters: parsed.outputParameters || [],
        metadata: parsed.metadata || {
          tags: [],
          category: 'imported'
        },
        createdBy: parsed.createdBy || 'imported',
        createdAt: parsed.createdAt || new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        executionCount: 0,
        successRate: 0,
        avgExecutionTime: undefined
      }

      return workflow
    } catch (error) {
      setValidationError(
        error instanceof Error ? error.message : 'Invalid workflow format'
      )
      return null
    }
  }

  // Handle file selection
  const handleFileSelect = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return

    const reader = new FileReader()
    reader.onload = (e) => {
      const content = e.target?.result as string
      setImportContent(content)
      setValidationError(null)
    }
    reader.onerror = () => {
      setValidationError('Failed to read file')
    }
    reader.readAsText(file)
  }

  // Handle import
  const handleImport = async () => {
    if (!importContent.trim()) {
      setValidationError('Please provide workflow content')
      return
    }

    setIsValidating(true)
    setValidationError(null)

    // Simulate validation delay
    await new Promise((resolve) => setTimeout(resolve, 500))

    const validatedWorkflow = validateWorkflow(importContent)

    if (validatedWorkflow && onImport) {
      onImport(validatedWorkflow)
      toast({
        title: 'Workflow imported successfully',
        description: `Imported "${validatedWorkflow.name}" v${validatedWorkflow.version}`
      })
      onOpenChange(false)
      setImportContent('')
    }

    setIsValidating(false)
  }

  // Handle export
  const handleExport = () => {
    if (!workflow) return

    const content = convertWorkflowToFormat(workflow, exportFormat)
    const blob = new Blob([content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${workflow.name.replace(/\s+/g, '_')}_v${workflow.version}.${
      exportFormat === 'yaml' ? 'yaml' : 'json'
    }`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)

    toast({
      title: 'Workflow exported',
      description: `Exported as ${exportFormat.toUpperCase()} format`
    })
  }

  // Handle copy to clipboard
  const handleCopyToClipboard = async () => {
    if (!workflow) return

    const content = convertWorkflowToFormat(workflow, exportFormat)

    try {
      await navigator.clipboard.writeText(content)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
      toast({
        title: 'Copied to clipboard',
        description: 'Workflow definition copied successfully'
      })
    } catch (error) {
      toast({
        title: 'Failed to copy',
        description: 'Unable to copy to clipboard',
        variant: 'destructive'
      })
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {mode === 'import' ? (
              <>
                <Upload className="h-5 w-5" />
                Import Workflow
              </>
            ) : (
              <>
                <Download className="h-5 w-5" />
                Export Workflow
              </>
            )}
          </DialogTitle>
          <DialogDescription>
            {mode === 'import'
              ? 'Import a workflow definition from a file or paste JSON/YAML content'
              : 'Export your workflow definition in various formats'}
          </DialogDescription>
        </DialogHeader>

        {mode === 'import' ? (
          <div className="space-y-4">
            {/* File Upload */}
            <div className="space-y-2">
              <Label>Upload File</Label>
              <div className="flex items-center gap-2">
                <Input
                  ref={fileInputRef}
                  type="file"
                  accept=".json,.yaml,.yml"
                  onChange={handleFileSelect}
                  className="hidden"
                />
                <Button
                  variant="outline"
                  onClick={() => fileInputRef.current?.click()}
                  className="w-full"
                >
                  <FileJson className="mr-2 h-4 w-4" />
                  Choose File
                </Button>
              </div>
            </div>

            {/* Or Divider */}
            <div className="relative">
              <div className="absolute inset-0 flex items-center">
                <span className="w-full border-t" />
              </div>
              <div className="relative flex justify-center text-xs uppercase">
                <span className="bg-background px-2 text-muted-foreground">
                  Or paste content
                </span>
              </div>
            </div>

            {/* Content Input */}
            <div className="space-y-2">
              <Label>Workflow Definition</Label>
              <Textarea
                placeholder="Paste your workflow JSON or YAML here..."
                className="min-h-[300px] font-mono text-sm"
                value={importContent}
                onChange={(e) => {
                  setImportContent(e.target.value)
                  setValidationError(null)
                }}
              />
            </div>

            {/* Validation Error */}
            {validationError && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{validationError}</AlertDescription>
              </Alert>
            )}
          </div>
        ) : (
          <div className="space-y-4">
            {/* Export Format */}
            <div className="space-y-2">
              <Label>Export Format</Label>
              <Select
                value={exportFormat}
                onValueChange={(value: ExportFormat) => setExportFormat(value)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="json">
                    <div className="flex items-center gap-2">
                      <FileJson className="h-4 w-4" />
                      JSON (Midaz Format)
                    </div>
                  </SelectItem>
                  <SelectItem value="yaml">
                    <div className="flex items-center gap-2">
                      <FileCode className="h-4 w-4" />
                      YAML
                    </div>
                  </SelectItem>
                  <SelectItem value="conductor">
                    <div className="flex items-center gap-2">
                      <FileJson className="h-4 w-4" />
                      Netflix Conductor Format
                    </div>
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>

            {/* Preview */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>Preview</Label>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleCopyToClipboard}
                  disabled={!workflow}
                >
                  {copied ? (
                    <>
                      <Check className="mr-2 h-4 w-4" />
                      Copied
                    </>
                  ) : (
                    <>
                      <Copy className="mr-2 h-4 w-4" />
                      Copy
                    </>
                  )}
                </Button>
              </div>
              <div className="overflow-auto rounded-md border bg-muted p-4">
                <pre className="text-sm">
                  {workflow
                    ? convertWorkflowToFormat(workflow, exportFormat)
                    : 'No workflow to export'}
                </pre>
              </div>
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          {mode === 'import' ? (
            <Button
              onClick={handleImport}
              disabled={!importContent.trim() || isValidating}
            >
              {isValidating ? 'Validating...' : 'Import Workflow'}
            </Button>
          ) : (
            <Button onClick={handleExport} disabled={!workflow}>
              <Download className="mr-2 h-4 w-4" />
              Export Workflow
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
