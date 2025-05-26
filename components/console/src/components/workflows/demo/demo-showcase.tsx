'use client'

import { useState } from 'react'
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
  GitBranch,
  Activity,
  BarChart3,
  Layers,
  CheckCircle,
  Clock,
  Users,
  Zap,
  Target,
  TrendingUp,
  Award,
  Info,
  Rocket
} from 'lucide-react'

interface DemoScenario {
  id: string
  title: string
  description: string
  category: string
  complexity: 'SIMPLE' | 'MEDIUM' | 'COMPLEX'
  duration: string
  steps: string[]
  highlights: string[]
}

export function DemoShowcase() {
  const [selectedScenario, setSelectedScenario] = useState<string | null>(null)

  const demoScenarios: DemoScenario[] = [
    {
      id: 'payment-flow',
      title: 'Payment Processing Workflow',
      description:
        'Complete end-to-end payment processing with validation, authorization, and settlement',
      category: 'Financial Operations',
      complexity: 'MEDIUM',
      duration: '3-5 minutes',
      steps: [
        'Template instantiation with payment parameters',
        'Visual workflow design and task configuration',
        'Execution monitoring with real-time updates',
        'Error handling and retry mechanisms',
        'Completion notification and audit trail'
      ],
      highlights: [
        'Netflix Conductor integration',
        'Real-time execution tracking',
        'Configurable task parameters',
        'Error handling and retries'
      ]
    },
    {
      id: 'onboarding-flow',
      title: 'Customer Onboarding Process',
      description:
        'Automated customer onboarding with KYC verification and account setup',
      category: 'Business Process',
      complexity: 'COMPLEX',
      duration: '5-8 minutes',
      steps: [
        'Multi-step workflow with human tasks',
        'KYC verification and decision points',
        'Parallel account creation processes',
        'Integration with external services',
        'Completion with welcome notification'
      ],
      highlights: [
        'Human task integration',
        'Decision nodes and branching',
        'Parallel task execution',
        'External service integration'
      ]
    },
    {
      id: 'reconciliation-demo',
      title: 'Daily Reconciliation Automation',
      description:
        'Automated daily reconciliation between internal and external systems',
      category: 'Automation',
      complexity: 'MEDIUM',
      duration: '4-6 minutes',
      steps: [
        'Scheduled workflow execution',
        'Data fetching from multiple sources',
        'Automated matching and exception detection',
        'Manual review for discrepancies',
        'Report generation and distribution'
      ],
      highlights: [
        'Scheduled execution',
        'Data integration',
        'Exception handling',
        'Automated reporting'
      ]
    },
    {
      id: 'analytics-demo',
      title: 'Advanced Analytics & Monitoring',
      description:
        'Comprehensive analytics dashboard with real-time monitoring capabilities',
      category: 'Analytics',
      complexity: 'SIMPLE',
      duration: '2-3 minutes',
      steps: [
        'Real-time dashboard exploration',
        'Performance metrics analysis',
        'Trend visualization and insights',
        'System health monitoring',
        'Alert configuration and management'
      ],
      highlights: [
        'Real-time dashboards',
        'Performance analytics',
        'Trend analysis',
        'Health monitoring'
      ]
    }
  ]

  const platformFeatures = [
    {
      icon: GitBranch,
      title: 'Visual Workflow Designer',
      description:
        'Drag-and-drop interface powered by React Flow for intuitive workflow design',
      color: 'blue'
    },
    {
      icon: Activity,
      title: 'Real-time Monitoring',
      description:
        'Live execution tracking with detailed timeline and status visualization',
      color: 'green'
    },
    {
      icon: Layers,
      title: 'Template Library',
      description:
        'Pre-built workflow templates for common financial and business processes',
      color: 'purple'
    },
    {
      icon: BarChart3,
      title: 'Advanced Analytics',
      description:
        'Comprehensive performance insights and execution trend analysis',
      color: 'orange'
    },
    {
      icon: Zap,
      title: 'Netflix Conductor',
      description:
        'Enterprise-grade orchestration engine for scalable workflow execution',
      color: 'red'
    },
    {
      icon: Users,
      title: 'Team Collaboration',
      description:
        'Multi-user workspace with role-based access and approval workflows',
      color: 'teal'
    }
  ]

  const getComplexityColor = (complexity: string) => {
    switch (complexity) {
      case 'SIMPLE':
        return 'bg-green-100 text-green-800'
      case 'MEDIUM':
        return 'bg-yellow-100 text-yellow-800'
      case 'COMPLEX':
        return 'bg-red-100 text-red-800'
      default:
        return 'bg-gray-100 text-gray-800'
    }
  }

  const getFeatureIconColor = (color: string) => {
    const colors = {
      blue: 'bg-blue-100 text-blue-600',
      green: 'bg-green-100 text-green-600',
      purple: 'bg-purple-100 text-purple-600',
      orange: 'bg-orange-100 text-orange-600',
      red: 'bg-red-100 text-red-600',
      teal: 'bg-teal-100 text-teal-600'
    }
    return colors[color as keyof typeof colors] || colors.blue
  }

  const runDemo = (scenarioId: string) => {
    setSelectedScenario(scenarioId)
    // In a real implementation, this would trigger the actual demo flow
    console.log(`Running demo scenario: ${scenarioId}`)
  }

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="text-center">
        <div className="mb-4 flex items-center justify-center gap-3">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-blue-100 text-blue-600">
            <Rocket className="h-6 w-6" />
          </div>
          <h1 className="text-3xl font-bold">Workflow Orchestration Demo</h1>
        </div>
        <p className="mx-auto max-w-2xl text-lg text-muted-foreground">
          Experience the power of Netflix Conductor-based workflow orchestration
          with visual design, real-time monitoring, and comprehensive analytics.
        </p>
      </div>

      {/* Demo Alert */}
      <Alert>
        <Info className="h-4 w-4" />
        <AlertDescription>
          This is a comprehensive demonstration of the Midaz Workflow
          Orchestration platform. All data shown is simulated for demonstration
          purposes.
        </AlertDescription>
      </Alert>

      <Tabs defaultValue="scenarios" className="w-full">
        <TabsList className="mx-auto grid w-full max-w-md grid-cols-3">
          <TabsTrigger value="scenarios">Demo Scenarios</TabsTrigger>
          <TabsTrigger value="features">Features</TabsTrigger>
          <TabsTrigger value="architecture">Architecture</TabsTrigger>
        </TabsList>

        <TabsContent value="scenarios" className="space-y-6">
          {/* Key Metrics */}
          <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
            <Card>
              <CardContent className="p-4 text-center">
                <div className="text-2xl font-bold text-blue-600">23</div>
                <div className="text-sm text-muted-foreground">
                  Active Workflows
                </div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4 text-center">
                <div className="text-2xl font-bold text-green-600">847</div>
                <div className="text-sm text-muted-foreground">
                  Executions Today
                </div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4 text-center">
                <div className="text-2xl font-bold text-purple-600">94.2%</div>
                <div className="text-sm text-muted-foreground">
                  Success Rate
                </div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4 text-center">
                <div className="text-2xl font-bold text-orange-600">3m 45s</div>
                <div className="text-sm text-muted-foreground">
                  Avg Duration
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Demo Scenarios */}
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            {demoScenarios.map((scenario) => (
              <Card
                key={scenario.id}
                className="transition-shadow hover:shadow-lg"
              >
                <CardHeader>
                  <div className="flex items-start justify-between">
                    <div className="flex-1">
                      <CardTitle className="text-lg">
                        {scenario.title}
                      </CardTitle>
                      <CardDescription className="mt-2">
                        {scenario.description}
                      </CardDescription>
                    </div>
                  </div>
                  <div className="mt-3 flex items-center gap-2">
                    <Badge variant="outline">{scenario.category}</Badge>
                    <Badge className={getComplexityColor(scenario.complexity)}>
                      {scenario.complexity}
                    </Badge>
                    <Badge variant="secondary">
                      <Clock className="mr-1 h-3 w-3" />
                      {scenario.duration}
                    </Badge>
                  </div>
                </CardHeader>
                <CardContent>
                  <div className="space-y-4">
                    {/* Demo Steps */}
                    <div>
                      <h4 className="mb-2 text-sm font-medium">Demo Flow:</h4>
                      <ol className="space-y-1 text-sm text-muted-foreground">
                        {scenario.steps.map((step, index) => (
                          <li key={index} className="flex items-start gap-2">
                            <span className="mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full bg-blue-100 text-xs font-medium text-blue-600">
                              {index + 1}
                            </span>
                            <span>{step}</span>
                          </li>
                        ))}
                      </ol>
                    </div>

                    {/* Highlights */}
                    <div>
                      <h4 className="mb-2 text-sm font-medium">
                        Key Features:
                      </h4>
                      <div className="flex flex-wrap gap-1">
                        {scenario.highlights.map((highlight) => (
                          <Badge
                            key={highlight}
                            variant="secondary"
                            className="text-xs"
                          >
                            {highlight}
                          </Badge>
                        ))}
                      </div>
                    </div>

                    {/* Action Button */}
                    <Button
                      onClick={() => runDemo(scenario.id)}
                      className="w-full"
                      variant={
                        selectedScenario === scenario.id ? 'default' : 'outline'
                      }
                    >
                      <Play className="mr-2 h-4 w-4" />
                      {selectedScenario === scenario.id
                        ? 'Demo Running...'
                        : 'Run Demo'}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </TabsContent>

        <TabsContent value="features" className="space-y-6">
          <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
            {platformFeatures.map((feature) => (
              <Card
                key={feature.title}
                className="transition-shadow hover:shadow-md"
              >
                <CardContent className="p-6">
                  <div className="flex items-start gap-4">
                    <div
                      className={`flex h-12 w-12 items-center justify-center rounded-xl ${getFeatureIconColor(feature.color)}`}
                    >
                      <feature.icon className="h-6 w-6" />
                    </div>
                    <div className="flex-1">
                      <h3 className="mb-2 font-semibold">{feature.title}</h3>
                      <p className="text-sm text-muted-foreground">
                        {feature.description}
                      </p>
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>

          {/* Feature Highlights */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Award className="h-5 w-5" />
                Platform Highlights
              </CardTitle>
              <CardDescription>
                Built for enterprise-scale workflow orchestration
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                <div className="space-y-3">
                  <div className="flex items-center gap-2">
                    <CheckCircle className="h-5 w-5 text-green-600" />
                    <span className="font-medium">
                      Enterprise-Grade Reliability
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    <CheckCircle className="h-5 w-5 text-green-600" />
                    <span className="font-medium">Visual Workflow Design</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <CheckCircle className="h-5 w-5 text-green-600" />
                    <span className="font-medium">Real-time Monitoring</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <CheckCircle className="h-5 w-5 text-green-600" />
                    <span className="font-medium">Template Library</span>
                  </div>
                </div>
                <div className="space-y-3">
                  <div className="flex items-center gap-2">
                    <CheckCircle className="h-5 w-5 text-green-600" />
                    <span className="font-medium">Advanced Analytics</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <CheckCircle className="h-5 w-5 text-green-600" />
                    <span className="font-medium">Scalable Architecture</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <CheckCircle className="h-5 w-5 text-green-600" />
                    <span className="font-medium">Team Collaboration</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <CheckCircle className="h-5 w-5 text-green-600" />
                    <span className="font-medium">API Integration</span>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="architecture" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>System Architecture</CardTitle>
              <CardDescription>
                Built on Netflix Conductor with modern web technologies
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-6">
                {/* Architecture Layers */}
                <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                  <Card>
                    <CardContent className="p-4 text-center">
                      <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-lg bg-blue-100 text-blue-600">
                        <GitBranch className="h-6 w-6" />
                      </div>
                      <h4 className="mb-2 font-medium">Frontend Layer</h4>
                      <p className="text-sm text-muted-foreground">
                        React + TypeScript with React Flow for visual design
                      </p>
                    </CardContent>
                  </Card>

                  <Card>
                    <CardContent className="p-4 text-center">
                      <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-lg bg-green-100 text-green-600">
                        <Zap className="h-6 w-6" />
                      </div>
                      <h4 className="mb-2 font-medium">Orchestration Engine</h4>
                      <p className="text-sm text-muted-foreground">
                        Netflix Conductor for reliable workflow execution
                      </p>
                    </CardContent>
                  </Card>

                  <Card>
                    <CardContent className="p-4 text-center">
                      <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-lg bg-purple-100 text-purple-600">
                        <BarChart3 className="h-6 w-6" />
                      </div>
                      <h4 className="mb-2 font-medium">
                        Analytics & Monitoring
                      </h4>
                      <p className="text-sm text-muted-foreground">
                        Real-time metrics and performance analytics
                      </p>
                    </CardContent>
                  </Card>
                </div>

                {/* Technical Stack */}
                <div>
                  <h4 className="mb-4 font-medium">Technology Stack</h4>
                  <div className="grid grid-cols-2 gap-4 text-sm md:grid-cols-4">
                    <div>
                      <div className="mb-2 font-medium text-blue-600">
                        Frontend
                      </div>
                      <ul className="space-y-1 text-muted-foreground">
                        <li>React 18</li>
                        <li>TypeScript</li>
                        <li>React Flow</li>
                        <li>Tailwind CSS</li>
                      </ul>
                    </div>
                    <div>
                      <div className="mb-2 font-medium text-green-600">
                        Backend
                      </div>
                      <ul className="space-y-1 text-muted-foreground">
                        <li>Netflix Conductor</li>
                        <li>REST APIs</li>
                        <li>WebSocket</li>
                        <li>PostgreSQL</li>
                      </ul>
                    </div>
                    <div>
                      <div className="mb-2 font-medium text-purple-600">
                        Infrastructure
                      </div>
                      <ul className="space-y-1 text-muted-foreground">
                        <li>Docker</li>
                        <li>Kubernetes</li>
                        <li>Redis</li>
                        <li>Elasticsearch</li>
                      </ul>
                    </div>
                    <div>
                      <div className="mb-2 font-medium text-orange-600">
                        Monitoring
                      </div>
                      <ul className="space-y-1 text-muted-foreground">
                        <li>Prometheus</li>
                        <li>Grafana</li>
                        <li>OpenTelemetry</li>
                        <li>Custom Metrics</li>
                      </ul>
                    </div>
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
