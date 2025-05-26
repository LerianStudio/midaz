'use client'

import React, { useEffect, useState } from 'react'
import { WorkflowCreationWizard } from '@/components/workflows/library/workflow-creation-wizard'
import { useRouter, useSearchParams } from 'next/navigation'
import { useToast } from '@/hooks/use-toast'
import { WorkflowTemplate } from '@/core/domain/entities/workflow-template'
import { mockWorkflowTemplates } from '@/core/domain/mock-data/workflow-templates'

export default function CreateWorkflowPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { toast } = useToast()
  const [initialTemplate, setInitialTemplate] = useState<
    WorkflowTemplate | undefined
  >()
  const [templateParameters, setTemplateParameters] = useState<
    Record<string, any> | undefined
  >()

  useEffect(() => {
    const templateId = searchParams.get('templateId')
    const params = searchParams.get('parameters')

    if (templateId) {
      // Find the template from mock data (in real app, fetch from API)
      const template = mockWorkflowTemplates.find((t) => t.id === templateId)
      if (template) {
        setInitialTemplate(template)

        // Parse parameters if provided
        if (params) {
          try {
            const parsedParams = JSON.parse(decodeURIComponent(params))
            setTemplateParameters(parsedParams)
          } catch (e) {
            console.error('Failed to parse template parameters:', e)
          }
        }
      } else {
        toast({
          title: 'Template not found',
          description: 'The specified template could not be found.',
          variant: 'destructive'
        })
      }
    }
  }, [searchParams, toast])

  const handleWorkflowCreate = (workflowData: any) => {
    // TODO: Integrate with workflow creation service
    console.log('Creating workflow:', workflowData)

    // Show success notification
    toast({
      title: 'Workflow created successfully',
      description: `${workflowData.name} has been created and is ready to use.`,
      variant: 'success'
    })

    // Navigate to the workflow designer with the template data
    // In a real implementation, you would use the created workflow ID
    const workflowId = 'new' // This will be replaced with actual ID after creation

    // If using a template, pass the template structure to the designer
    if (workflowData.useTemplate && workflowData.templateTasks) {
      // Store template data in session storage for the designer to pick up
      sessionStorage.setItem(
        `workflow-template-${workflowId}`,
        JSON.stringify({
          name: workflowData.name,
          description: workflowData.description,
          tasks: workflowData.templateTasks,
          inputParameters: workflowData.inputParameters,
          outputParameters: workflowData.outputParameters,
          metadata: {
            templateId: workflowData.templateId,
            templateParameters: workflowData.templateParameters,
            tags: workflowData.tags
          }
        })
      )
    }

    router.push(`/plugins/workflows/library/${workflowId}/designer`)
  }

  const handleCancel = () => {
    router.push('/plugins/workflows/library')
  }

  return (
    <div className="container mx-auto max-w-5xl p-6">
      <div className="mb-6">
        <h1 className="text-3xl font-bold">Create New Workflow</h1>
        <p className="mt-2 text-muted-foreground">
          Follow the steps below to create a new workflow for your organization
        </p>
      </div>

      <WorkflowCreationWizard
        onSubmit={handleWorkflowCreate}
        onCancel={handleCancel}
        initialTemplate={initialTemplate}
        initialTemplateParameters={templateParameters}
      />
    </div>
  )
}
