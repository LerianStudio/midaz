'use client'

import React from 'react'
import { useRouter } from 'next/navigation'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Play,
  Zap,
  Users,
  CreditCard,
  CheckCircle,
  GitBranch,
  Activity,
  Shield,
  DollarSign,
  FileText,
  AlertTriangle,
  ArrowRight,
  Sparkles
} from 'lucide-react'
import { crossServiceTemplates } from '@/core/domain/mock-data/cross-service-templates'

interface DemoScenario {
  id: string
  title: string
  description: string
  icon: React.ReactNode
  services: string[]
  estimatedTime: string
  complexity: 'SIMPLE' | 'MEDIUM' | 'COMPLEX' | 'ADVANCED'
  features: string[]
  templateId?: string
}

const demoScenarios: DemoScenario[] = [
  {
    id: 'customer-onboarding',
    title: 'Complete Customer Onboarding',
    description:
      'End-to-end customer onboarding with KYC verification, account creation, and multi-service orchestration',
    icon: <Users className="h-6 w-6" />,
    services: ['Identity', 'CRM', 'Auth', 'Onboarding', 'Notifications'],
    estimatedTime: '5-10 minutes',
    complexity: 'COMPLEX',
    features: [
      'Automated KYC verification',
      'Conditional workflow routing',
      'Multi-service coordination',
      'Error handling and retries',
      'Human task for manual review'
    ],
    templateId: 'template-customer-onboarding-complete'
  },
  {
    id: 'payment-processing',
    title: 'Smart Payment Processing',
    description:
      'Process payments with dynamic fee calculation, fraud detection, and real-time notifications',
    icon: <CreditCard className="h-6 w-6" />,
    services: [
      'Transaction',
      'Fees',
      'Fraud Detection',
      'Notifications',
      'Compliance'
    ],
    estimatedTime: '2-5 minutes',
    complexity: 'ADVANCED',
    features: [
      'Dynamic fee calculation',
      'Real-time fraud scoring',
      'Multi-currency support',
      'Parallel notification delivery',
      'Compliance reporting'
    ],
    templateId: 'template-payment-with-fees'
  },
  {
    id: 'reconciliation',
    title: 'Automated Reconciliation',
    description:
      'Reconcile transactions from multiple sources with intelligent matching and exception handling',
    icon: <FileText className="h-6 w-6" />,
    services: [
      'Reconciliation',
      'Transaction',
      'Reporting',
      'Document Storage'
    ],
    estimatedTime: '15-30 minutes',
    complexity: 'ADVANCED',
    features: [
      'Multi-source data fetching',
      'Intelligent transaction matching',
      'Automatic exception routing',
      'Adjustment creation',
      'Comprehensive reporting'
    ],
    templateId: 'template-reconciliation-workflow'
  }
]

const getComplexityColor = (complexity: string) => {
  switch (complexity) {
    case 'SIMPLE':
      return 'bg-green-100 text-green-800'
    case 'MEDIUM':
      return 'bg-yellow-100 text-yellow-800'
    case 'COMPLEX':
      return 'bg-orange-100 text-orange-800'
    case 'ADVANCED':
      return 'bg-red-100 text-red-800'
    default:
      return 'bg-gray-100 text-gray-800'
  }
}

export default function WorkflowDemoPage() {
  const router = useRouter()

  const handleStartDemo = (scenario: DemoScenario) => {
    if (scenario.templateId) {
      // Navigate to create workflow with the template
      router.push(
        `/plugins/workflows/library/create?templateId=${scenario.templateId}`
      )
    }
  }

  const handleViewTemplate = (templateId: string) => {
    // Navigate to template details
    router.push(`/plugins/workflows/library/templates/${templateId}`)
  }

  return (
    <div className="space-y-6 p-6">
      {/* Header */}
      <div className="space-y-2">
        <div className="flex items-center gap-2">
          <h1 className="text-3xl font-bold">Workflow Orchestration Demos</h1>
          <Badge variant="secondary" className="gap-1">
            <Sparkles className="h-3 w-3" />
            Interactive
          </Badge>
        </div>
        <p className="text-muted-foreground">
          Experience the power of cross-service orchestration with these
          real-world scenarios
        </p>
      </div>

      {/* Key Features */}
      <Card className="border-primary/20 bg-primary/5">
        <CardHeader>
          <CardTitle className="text-lg">🚀 Platform Capabilities</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <div className="flex items-start gap-3">
              <div className="rounded-full bg-primary/10 p-2">
                <GitBranch className="h-4 w-4 text-primary" />
              </div>
              <div>
                <h4 className="font-medium">Multi-Service Orchestration</h4>
                <p className="text-sm text-muted-foreground">
                  Coordinate actions across all Midaz services seamlessly
                </p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <div className="rounded-full bg-primary/10 p-2">
                <Activity className="h-4 w-4 text-primary" />
              </div>
              <div>
                <h4 className="font-medium">Real-time Monitoring</h4>
                <p className="text-sm text-muted-foreground">
                  Track execution progress with live updates via WebSocket
                </p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <div className="rounded-full bg-primary/10 p-2">
                <Shield className="h-4 w-4 text-primary" />
              </div>
              <div>
                <h4 className="font-medium">Enterprise-Grade</h4>
                <p className="text-sm text-muted-foreground">
                  Built on Netflix Conductor for reliability at scale
                </p>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Demo Scenarios */}
      <div>
        <h2 className="mb-4 text-xl font-semibold">Choose a Demo Scenario</h2>
        <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
          {demoScenarios.map((scenario) => (
            <Card
              key={scenario.id}
              className="transition-shadow hover:shadow-lg"
            >
              <CardHeader>
                <div className="mb-2 flex items-start justify-between">
                  <div className="rounded-lg bg-primary/10 p-3">
                    {scenario.icon}
                  </div>
                  <Badge className={getComplexityColor(scenario.complexity)}>
                    {scenario.complexity}
                  </Badge>
                </div>
                <CardTitle className="text-lg">{scenario.title}</CardTitle>
                <CardDescription>{scenario.description}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {/* Services */}
                <div>
                  <p className="mb-2 text-sm font-medium">Services Involved</p>
                  <div className="flex flex-wrap gap-1">
                    {scenario.services.map((service) => (
                      <Badge
                        key={service}
                        variant="outline"
                        className="text-xs"
                      >
                        {service}
                      </Badge>
                    ))}
                  </div>
                </div>

                {/* Features */}
                <div>
                  <p className="mb-2 text-sm font-medium">Key Features</p>
                  <ul className="space-y-1 text-sm text-muted-foreground">
                    {scenario.features.slice(0, 3).map((feature, index) => (
                      <li key={index} className="flex items-center gap-2">
                        <CheckCircle className="h-3 w-3 text-green-600" />
                        {feature}
                      </li>
                    ))}
                  </ul>
                </div>

                {/* Time estimate */}
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Activity className="h-3 w-3" />
                  <span>{scenario.estimatedTime}</span>
                </div>

                {/* Actions */}
                <div className="flex gap-2">
                  <Button
                    onClick={() => handleStartDemo(scenario)}
                    className="flex-1"
                    size="sm"
                  >
                    <Play className="mr-2 h-3 w-3" />
                    Start Demo
                  </Button>
                  {scenario.templateId && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleViewTemplate(scenario.templateId!)}
                    >
                      View Details
                    </Button>
                  )}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>

      {/* Live Examples */}
      <Card>
        <CardHeader>
          <CardTitle>💡 Try These Scenarios</CardTitle>
          <CardDescription>
            Real-world use cases demonstrating the power of workflow
            orchestration
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue="onboarding" className="w-full">
            <TabsList className="grid w-full grid-cols-3">
              <TabsTrigger value="onboarding">Onboarding</TabsTrigger>
              <TabsTrigger value="payment">Payment</TabsTrigger>
              <TabsTrigger value="reconciliation">Reconciliation</TabsTrigger>
            </TabsList>

            <TabsContent value="onboarding" className="space-y-4">
              <Alert>
                <Users className="h-4 w-4" />
                <AlertDescription>
                  <strong>Scenario:</strong> A new business customer needs to be
                  onboarded with full KYC verification
                </AlertDescription>
              </Alert>
              <div className="space-y-2">
                <h4 className="font-medium">Workflow Steps:</h4>
                <ol className="list-inside list-decimal space-y-1 text-sm text-muted-foreground">
                  <li>Identity verification with government ID check</li>
                  <li>Business registration validation</li>
                  <li>CRM record creation with customer profile</li>
                  <li>Authentication user setup with secure credentials</li>
                  <li>Organization and ledger creation</li>
                  <li>Primary business account setup</li>
                  <li>Welcome email with portal access</li>
                </ol>
              </div>
              <Button onClick={() => handleStartDemo(demoScenarios[0])}>
                <Zap className="mr-2 h-4 w-4" />
                Run Onboarding Demo
              </Button>
            </TabsContent>

            <TabsContent value="payment" className="space-y-4">
              <Alert>
                <CreditCard className="h-4 w-4" />
                <AlertDescription>
                  <strong>Scenario:</strong> Process a cross-border payment with
                  dynamic fees and fraud detection
                </AlertDescription>
              </Alert>
              <div className="space-y-2">
                <h4 className="font-medium">Workflow Steps:</h4>
                <ol className="list-inside list-decimal space-y-1 text-sm text-muted-foreground">
                  <li>Validate source and destination accounts</li>
                  <li>Real-time fraud scoring and decision</li>
                  <li>Dynamic fee calculation based on amount and type</li>
                  <li>Currency conversion with live rates</li>
                  <li>Transaction creation and processing</li>
                  <li>Balance updates across accounts</li>
                  <li>Multi-channel notifications</li>
                </ol>
              </div>
              <Button onClick={() => handleStartDemo(demoScenarios[1])}>
                <Zap className="mr-2 h-4 w-4" />
                Run Payment Demo
              </Button>
            </TabsContent>

            <TabsContent value="reconciliation" className="space-y-4">
              <Alert>
                <FileText className="h-4 w-4" />
                <AlertDescription>
                  <strong>Scenario:</strong> Daily reconciliation of
                  transactions across multiple banks and payment processors
                </AlertDescription>
              </Alert>
              <div className="space-y-2">
                <h4 className="font-medium">Workflow Steps:</h4>
                <ol className="list-inside list-decimal space-y-1 text-sm text-muted-foreground">
                  <li>Fetch internal transaction records</li>
                  <li>Connect to multiple external sources in parallel</li>
                  <li>Transform and normalize data formats</li>
                  <li>Run intelligent matching algorithms</li>
                  <li>Analyze and categorize exceptions</li>
                  <li>Create adjustment entries as needed</li>
                  <li>Generate comprehensive reports</li>
                </ol>
              </div>
              <Button onClick={() => handleStartDemo(demoScenarios[2])}>
                <Zap className="mr-2 h-4 w-4" />
                Run Reconciliation Demo
              </Button>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      {/* Call to Action */}
      <Card className="border-primary/20 bg-gradient-to-r from-primary/10 to-primary/5">
        <CardContent className="flex items-center justify-between p-6">
          <div className="space-y-1">
            <h3 className="text-lg font-semibold">
              Ready to build your own workflows?
            </h3>
            <p className="text-sm text-muted-foreground">
              Create custom workflows using our visual designer or start from a
              template
            </p>
          </div>
          <div className="flex gap-2">
            <Button
              variant="outline"
              onClick={() => router.push('/plugins/workflows/library')}
            >
              Browse Templates
            </Button>
            <Button
              onClick={() => router.push('/plugins/workflows/library/create')}
            >
              Create Workflow
              <ArrowRight className="ml-2 h-4 w-4" />
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
