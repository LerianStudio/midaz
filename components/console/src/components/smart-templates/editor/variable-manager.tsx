'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import { Textarea } from '@/components/ui/textarea'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Plus,
  Edit,
  Trash2,
  Braces,
  Type,
  Calendar,
  Hash,
  ToggleLeft,
  Database,
  Copy,
  Check
} from 'lucide-react'

interface TemplateVariable {
  id: string
  name: string
  type: 'string' | 'number' | 'date' | 'boolean' | 'array' | 'object'
  description: string
  defaultValue?: any
  required: boolean
  validation?: {
    pattern?: string
    min?: number
    max?: number
    options?: string[]
  }
  dataSource?: string
}

interface VariableManagerProps {
  variables: TemplateVariable[]
  onVariablesChange: (variables: TemplateVariable[]) => void
  onInsertVariable: (variableName: string) => void
}

export function VariableManager({
  variables,
  onVariablesChange,
  onInsertVariable
}: VariableManagerProps) {
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false)
  const [editingVariable, setEditingVariable] =
    useState<TemplateVariable | null>(null)
  const [newVariable, setNewVariable] = useState<Partial<TemplateVariable>>({
    name: '',
    type: 'string',
    description: '',
    required: false
  })
  const [copiedVariable, setCopiedVariable] = useState<string | null>(null)

  const typeIcons = {
    string: <Type className="h-4 w-4" />,
    number: <Hash className="h-4 w-4" />,
    date: <Calendar className="h-4 w-4" />,
    boolean: <Toggle className="h-4 w-4" />,
    array: <Database className="h-4 w-4" />,
    object: <Braces className="h-4 w-4" />
  }

  const typeColors = {
    string: 'bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-200',
    number: 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200',
    date: 'bg-purple-100 text-purple-800 dark:bg-purple-800 dark:text-purple-200',
    boolean:
      'bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-200',
    array:
      'bg-orange-100 text-orange-800 dark:bg-orange-800 dark:text-orange-200',
    object: 'bg-pink-100 text-pink-800 dark:bg-pink-800 dark:text-pink-200'
  }

  const handleCreateVariable = () => {
    if (newVariable.name && newVariable.type && newVariable.description) {
      const variable: TemplateVariable = {
        id: `var-${Date.now()}`,
        name: newVariable.name,
        type: newVariable.type as TemplateVariable['type'],
        description: newVariable.description,
        required: newVariable.required || false,
        defaultValue: newVariable.defaultValue,
        validation: newVariable.validation
      }

      onVariablesChange([...variables, variable])
      setNewVariable({
        name: '',
        type: 'string',
        description: '',
        required: false
      })
      setIsCreateDialogOpen(false)
    }
  }

  const handleEditVariable = (variable: TemplateVariable) => {
    setEditingVariable(variable)
    setNewVariable({ ...variable })
    setIsCreateDialogOpen(true)
  }

  const handleUpdateVariable = () => {
    if (
      editingVariable &&
      newVariable.name &&
      newVariable.type &&
      newVariable.description
    ) {
      const updatedVariables = variables.map((v) =>
        v.id === editingVariable.id
          ? ({ ...editingVariable, ...newVariable } as TemplateVariable)
          : v
      )
      onVariablesChange(updatedVariables)
      setEditingVariable(null)
      setNewVariable({
        name: '',
        type: 'string',
        description: '',
        required: false
      })
      setIsCreateDialogOpen(false)
    }
  }

  const handleDeleteVariable = (variableId: string) => {
    const updatedVariables = variables.filter((v) => v.id !== variableId)
    onVariablesChange(updatedVariables)
  }

  const handleCopyVariable = (variableName: string) => {
    navigator.clipboard.writeText(`{{${variableName}}}`)
    setCopiedVariable(variableName)
    setTimeout(() => setCopiedVariable(null), 2000)
  }

  const renderVariableDialog = () => (
    <Dialog open={isCreateDialogOpen} onOpenChange={setIsCreateDialogOpen}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {editingVariable ? 'Edit Variable' : 'Create New Variable'}
          </DialogTitle>
          <DialogDescription>
            {editingVariable
              ? 'Update the variable properties below.'
              : 'Define a new template variable that can be used in your template.'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="variable-name">Variable Name *</Label>
            <Input
              id="variable-name"
              placeholder="e.g., user_name, total_amount"
              value={newVariable.name || ''}
              onChange={(e) =>
                setNewVariable({ ...newVariable, name: e.target.value })
              }
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="variable-type">Type *</Label>
            <Select
              value={newVariable.type}
              onValueChange={(value: TemplateVariable['type']) =>
                setNewVariable({ ...newVariable, type: value })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="string">Text (String)</SelectItem>
                <SelectItem value="number">Number</SelectItem>
                <SelectItem value="date">Date</SelectItem>
                <SelectItem value="boolean">Boolean</SelectItem>
                <SelectItem value="array">Array</SelectItem>
                <SelectItem value="object">Object</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="variable-description">Description *</Label>
            <Textarea
              id="variable-description"
              placeholder="Describe what this variable represents..."
              value={newVariable.description || ''}
              onChange={(e) =>
                setNewVariable({ ...newVariable, description: e.target.value })
              }
              rows={2}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="default-value">Default Value (Optional)</Label>
            <Input
              id="default-value"
              placeholder="Default value for this variable"
              value={newVariable.defaultValue || ''}
              onChange={(e) =>
                setNewVariable({ ...newVariable, defaultValue: e.target.value })
              }
            />
          </div>

          <div className="flex items-center space-x-2">
            <input
              type="checkbox"
              id="required"
              checked={newVariable.required || false}
              onChange={(e) =>
                setNewVariable({ ...newVariable, required: e.target.checked })
              }
              className="rounded border-gray-300"
            />
            <Label htmlFor="required">Required variable</Label>
          </div>
        </div>

        <div className="flex justify-end space-x-2 pt-4">
          <Button
            variant="outline"
            onClick={() => {
              setIsCreateDialogOpen(false)
              setEditingVariable(null)
              setNewVariable({
                name: '',
                type: 'string',
                description: '',
                required: false
              })
            }}
          >
            Cancel
          </Button>
          <Button
            onClick={
              editingVariable ? handleUpdateVariable : handleCreateVariable
            }
            disabled={!newVariable.name || !newVariable.description}
          >
            {editingVariable ? 'Update' : 'Create'} Variable
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-medium">Template Variables</h3>
        <Dialog open={isCreateDialogOpen} onOpenChange={setIsCreateDialogOpen}>
          <DialogTrigger asChild>
            <Button size="sm" className="flex items-center space-x-2">
              <Plus className="h-4 w-4" />
              <span>Add Variable</span>
            </Button>
          </DialogTrigger>
        </Dialog>
      </div>

      <ScrollArea className="h-[400px]">
        <div className="space-y-2">
          {variables.length === 0 ? (
            <Card>
              <CardContent className="p-6 text-center">
                <Braces className="mx-auto mb-2 h-8 w-8 text-muted-foreground" />
                <p className="text-sm text-muted-foreground">
                  No variables defined yet. Create your first variable to get
                  started.
                </p>
              </CardContent>
            </Card>
          ) : (
            variables.map((variable) => (
              <Card key={variable.id} className="p-3">
                <div className="flex items-start justify-between">
                  <div className="flex flex-1 items-start space-x-3">
                    <div className="mt-1">{typeIcons[variable.type]}</div>
                    <div className="min-w-0 flex-1">
                      <div className="mb-1 flex items-center space-x-2">
                        <h4 className="text-sm font-medium">{variable.name}</h4>
                        <Badge
                          className={typeColors[variable.type]}
                          variant="secondary"
                        >
                          {variable.type}
                        </Badge>
                        {variable.required && (
                          <Badge variant="destructive" className="text-xs">
                            Required
                          </Badge>
                        )}
                      </div>
                      <p className="mb-2 text-xs text-muted-foreground">
                        {variable.description}
                      </p>
                      {variable.defaultValue && (
                        <p className="text-xs text-muted-foreground">
                          Default:{' '}
                          <code className="rounded bg-muted px-1 text-xs">
                            {variable.defaultValue}
                          </code>
                        </p>
                      )}
                    </div>
                  </div>

                  <div className="flex items-center space-x-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => {
                        onInsertVariable(variable.name)
                        handleCopyVariable(variable.name)
                      }}
                      className="h-8 w-8 p-0"
                      title="Insert variable"
                    >
                      {copiedVariable === variable.name ? (
                        <Check className="h-3 w-3 text-green-600" />
                      ) : (
                        <Copy className="h-3 w-3" />
                      )}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleEditVariable(variable)}
                      className="h-8 w-8 p-0"
                      title="Edit variable"
                    >
                      <Edit className="h-3 w-3" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleDeleteVariable(variable.id)}
                      className="h-8 w-8 p-0 text-red-600 hover:text-red-700"
                      title="Delete variable"
                    >
                      <Trash2 className="h-3 w-3" />
                    </Button>
                  </div>
                </div>
              </Card>
            ))
          )}
        </div>
      </ScrollArea>

      {/* Predefined Variables */}
      <div className="space-y-2">
        <h4 className="text-sm font-medium">System Variables</h4>
        <div className="space-y-1">
          {[
            { name: 'current_date', description: 'Current date and time' },
            {
              name: 'generated_by',
              description: 'User who generated the report'
            },
            {
              name: 'template_version',
              description: 'Template version number'
            },
            { name: 'organization_name', description: 'Organization name' }
          ].map((sysVar) => (
            <div
              key={sysVar.name}
              className="flex cursor-pointer items-center justify-between rounded-md bg-muted/30 p-2 hover:bg-muted/50"
              onClick={() => onInsertVariable(sysVar.name)}
            >
              <div className="flex items-center space-x-2">
                <Braces className="h-3 w-3 text-muted-foreground" />
                <div>
                  <span className="text-sm font-medium">{sysVar.name}</span>
                  <p className="text-xs text-muted-foreground">
                    {sysVar.description}
                  </p>
                </div>
              </div>
              <Badge variant="outline" className="text-xs">
                System
              </Badge>
            </div>
          ))}
        </div>
      </div>

      {renderVariableDialog()}
    </div>
  )
}
