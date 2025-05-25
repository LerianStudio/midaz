'use client'

import { useRouter } from 'next/navigation'
import { ArrowLeft } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/page-header'

import { OperationRouteForm } from '@/components/accounting/operation-routes/operation-route-form'
import { type OperationRoute } from '@/components/accounting/mock/transaction-route-mock-data'

export default function CreateOperationRoutePage() {
  const router = useRouter()

  const handleSave = (operation: OperationRoute) => {
    // In a real implementation, this would call an API to create the operation
    console.log('Creating operation route:', operation)

    // Redirect back to the operations list
    router.push('/plugins/accounting/operation-routes')
  }

  const handleCancel = () => {
    router.push('/plugins/accounting/operation-routes')
  }

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <div className="flex items-center space-x-4">
          <Button variant="ghost" size="sm" onClick={() => router.back()}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <PageHeader.InfoTitle>Create Operation Route</PageHeader.InfoTitle>
            <PageHeader.InfoTooltip>
              Create a new operation route mapping between account types.
            </PageHeader.InfoTooltip>
          </div>
        </div>
      </PageHeader.Root>

      <OperationRouteForm
        onSave={handleSave}
        onCancel={handleCancel}
        mode="create"
      />
    </div>
  )
}
