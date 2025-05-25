'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { 
  UserCheck, 
  FileText, 
  CheckCircle, 
  AlertTriangle, 
  Clock, 
  Shield,
  BarChart3,
  Upload
} from 'lucide-react'

export default function IdentityPage() {
  const intl = useIntl()

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Identity Verification</h1>
        <p className="text-muted-foreground">
          KYC/AML compliance, identity verification workflows, and document validation processes.
        </p>
      </div>

      {/* Quick Stats */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card className="p-6">
          <div className="flex items-center">
            <CheckCircle className="h-8 w-8 text-green-600" />
            <div className="ml-4">
              <p className="text-sm font-medium text-muted-foreground">Verified</p>
              <p className="text-2xl font-bold">1,845</p>
            </div>
          </div>
        </Card>
        <Card className="p-6">
          <div className="flex items-center">
            <Clock className="h-8 w-8 text-yellow-600" />
            <div className="ml-4">
              <p className="text-sm font-medium text-muted-foreground">Pending</p>
              <p className="text-2xl font-bold">123</p>
            </div>
          </div>
        </Card>
        <Card className="p-6">
          <div className="flex items-center">
            <AlertTriangle className="h-8 w-8 text-red-600" />
            <div className="ml-4">
              <p className="text-sm font-medium text-muted-foreground">Rejected</p>
              <p className="text-2xl font-bold">45</p>
            </div>
          </div>
        </Card>
        <Card className="p-6">
          <div className="flex items-center">
            <Shield className="h-8 w-8 text-blue-600" />
            <div className="ml-4">
              <p className="text-sm font-medium text-muted-foreground">Compliance Score</p>
              <p className="text-2xl font-bold">96%</p>
            </div>
          </div>
        </Card>
      </div>

      {/* Verification Workflows */}
      <div className="grid gap-6 md:grid-cols-2">
        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-lg font-semibold">KYC Verification</h3>
              <p className="text-sm text-muted-foreground">
                Know Your Customer identity verification process
              </p>
            </div>
            <Button>
              <UserCheck className="mr-2 h-4 w-4" />
              Start KYC
            </Button>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-lg font-semibold">Document Upload</h3>
              <p className="text-sm text-muted-foreground">
                Upload and validate identity documents
              </p>
            </div>
            <Button>
              <Upload className="mr-2 h-4 w-4" />
              Upload Docs
            </Button>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-lg font-semibold">AML Screening</h3>
              <p className="text-sm text-muted-foreground">
                Anti-Money Laundering compliance checks
              </p>
            </div>
            <Button>
              <Shield className="mr-2 h-4 w-4" />
              AML Check
            </Button>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-lg font-semibold">Analytics</h3>
              <p className="text-sm text-muted-foreground">
                View verification metrics and compliance reports
              </p>
            </div>
            <Button>
              <BarChart3 className="mr-2 h-4 w-4" />
              View Reports
            </Button>
          </div>
        </Card>
      </div>

      {/* Recent Verifications */}
      <Card>
        <div className="p-6">
          <h3 className="text-lg font-semibold mb-4">Recent Verification Requests</h3>
          <div className="space-y-4">
            <div className="flex items-center justify-between border-b pb-2">
              <div className="flex items-center gap-3">
                <UserCheck className="h-5 w-5 text-muted-foreground" />
                <div>
                  <p className="font-medium">Individual KYC</p>
                  <p className="text-sm text-muted-foreground">John Smith - ID: KYC-001234</p>
                </div>
              </div>
              <Badge variant="default" className="bg-green-100 text-green-800">
                Verified
              </Badge>
            </div>
            <div className="flex items-center justify-between border-b pb-2">
              <div className="flex items-center gap-3">
                <FileText className="h-5 w-5 text-muted-foreground" />
                <div>
                  <p className="font-medium">Document Review</p>
                  <p className="text-sm text-muted-foreground">Corporate Registration - ID: DOC-005678</p>
                </div>
              </div>
              <Badge variant="secondary" className="bg-yellow-100 text-yellow-800">
                Pending
              </Badge>
            </div>
            <div className="flex items-center justify-between border-b pb-2">
              <div className="flex items-center gap-3">
                <Shield className="h-5 w-5 text-muted-foreground" />
                <div>
                  <p className="font-medium">AML Screening</p>
                  <p className="text-sm text-muted-foreground">Sarah Johnson - ID: AML-002345</p>
                </div>
              </div>
              <Badge variant="destructive" className="bg-red-100 text-red-800">
                Flagged
              </Badge>
            </div>
            <div className="flex items-center justify-between border-b pb-2">
              <div className="flex items-center gap-3">
                <UserCheck className="h-5 w-5 text-muted-foreground" />
                <div>
                  <p className="font-medium">Enhanced DD</p>
                  <p className="text-sm text-muted-foreground">TechCorp Ltd - ID: EDD-007890</p>
                </div>
              </div>
              <Badge variant="default" className="bg-blue-100 text-blue-800">
                In Progress
              </Badge>
            </div>
          </div>
        </div>
      </Card>
    </div>
  )
}