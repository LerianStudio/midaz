'use client'

import { useParams } from 'next/navigation'
import Link from 'next/link'
import { ArrowLeft } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

export default function ProcessMonitoringPage() {
  const params = useParams()
  const processId = params.id as string

  return (
    <div className="container mx-auto space-y-6 p-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href={`/plugins/reconciliation/processes/${processId}`}>
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-1 h-4 w-4" />
              Back
            </Button>
          </Link>
          <h1 className="text-2xl font-bold">Process Monitoring</h1>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Real-time Monitoring</CardTitle>
        </CardHeader>
        <CardContent>
          <p>
            Monitoring functionality temporarily simplified for build process.
          </p>
          <p>Process ID: {processId}</p>
        </CardContent>
      </Card>
    </div>
  )
}
