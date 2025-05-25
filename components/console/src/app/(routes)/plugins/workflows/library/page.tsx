import React from 'react'
import { Metadata } from 'next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { WorkflowListTable } from '@/components/workflows/library/workflow-list-table'
import { TemplateCatalog } from '@/components/workflows/templates/template-catalog'

export const metadata: Metadata = {
  title: 'Workflow Library - Workflows',
  description: 'Manage and organize your workflow definitions'
}

export default function WorkflowLibraryPage() {
  const handleUseTemplate = (
    template: any,
    parameters: Record<string, any>
  ) => {
    // TODO: Integrate with workflow creation service
    console.log('Creating workflow from template:', template.name, parameters)

    // Here you would typically:
    // 1. Call the workflow service to create a new workflow
    // 2. Navigate to the workflow designer with the new workflow
    // 3. Show success notification
  }

  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="text-2xl font-bold">Workflow Library</h1>
        <p className="text-muted-foreground">
          Manage your workflow definitions and discover pre-built templates
        </p>
      </div>

      <Tabs defaultValue="workflows" className="w-full">
        <TabsList className="mb-6">
          <TabsTrigger value="workflows">My Workflows</TabsTrigger>
          <TabsTrigger value="templates">Templates</TabsTrigger>
        </TabsList>

        <TabsContent value="workflows" className="mt-0">
          <WorkflowListTable />
        </TabsContent>

        <TabsContent value="templates" className="mt-0">
          <TemplateCatalog onUseTemplate={handleUseTemplate} />
        </TabsContent>
      </Tabs>
    </div>
  )
}
