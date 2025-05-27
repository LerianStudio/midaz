'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import {
  Sparkles,
  FileText,
  BarChart3,
  Database,
  Zap,
  ArrowRight,
  CheckCircle,
  Play,
  Eye,
  Download
} from 'lucide-react'

interface DemoStep {
  id: string
  title: string
  description: string
  icon: React.ReactNode
  action: string
  route: string
  completed?: boolean
}

const demoSteps: DemoStep[] = [
  {
    id: 'overview',
    title: 'Smart Templates Overview',
    description:
      'See the power of automated report generation with real-time metrics',
    icon: <BarChart3 className="h-5 w-5" />,
    action: 'View Dashboard',
    route: '/plugins/smart-templates'
  },
  {
    id: 'templates',
    title: 'Browse Template Library',
    description:
      'Explore pre-built templates for financial reports, compliance, and analytics',
    icon: <FileText className="h-5 w-5" />,
    action: 'Browse Templates',
    route: '/plugins/smart-templates/templates'
  },
  {
    id: 'create',
    title: 'Create Your First Template',
    description:
      'Use our wizard to create a custom template with data sources and variables',
    icon: <Sparkles className="h-5 w-5" />,
    action: 'Create Template',
    route: '/plugins/smart-templates/templates/create'
  },
  {
    id: 'editor',
    title: 'Template Editor Experience',
    description:
      'Experience our powerful editor with live preview and variable management',
    icon: <Eye className="h-5 w-5" />,
    action: 'Try Editor',
    route: '/plugins/smart-templates/templates/tpl-1/edit'
  },
  {
    id: 'generate',
    title: 'Generate Your First Report',
    description:
      'Generate a sample report and see the real-time monitoring in action',
    icon: <Play className="h-5 w-5" />,
    action: 'Generate Report',
    route: '/plugins/smart-templates/reports/generate'
  },
  {
    id: 'monitor',
    title: 'Monitor Report Generation',
    description: 'Track report generation jobs and download completed reports',
    icon: <Download className="h-5 w-5" />,
    action: 'View Reports',
    route: '/plugins/smart-templates/reports'
  },
  {
    id: 'analytics',
    title: 'Analytics & Insights',
    description:
      'Discover performance metrics and usage analytics for optimization',
    icon: <BarChart3 className="h-5 w-5" />,
    action: 'View Analytics',
    route: '/plugins/smart-templates/analytics'
  },
  {
    id: 'datasources',
    title: 'Data Source Management',
    description: 'Connect to various data sources for dynamic report content',
    icon: <Database className="h-5 w-5" />,
    action: 'Manage Sources',
    route: '/plugins/smart-templates/data-sources'
  }
]

export function SmartTemplatesDemoWizard() {
  const router = useRouter()
  const [currentStep, setCurrentStep] = useState(0)
  const [completedSteps, setCompletedSteps] = useState<string[]>([])

  const progress = (completedSteps.length / demoSteps.length) * 100

  const handleStepClick = (step: DemoStep, index: number) => {
    setCurrentStep(index)
    router.push(step.route)

    // Mark step as completed
    if (!completedSteps.includes(step.id)) {
      setCompletedSteps([...completedSteps, step.id])
    }
  }

  const isStepCompleted = (stepId: string) => completedSteps.includes(stepId)

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header */}
      <div className="space-y-4 text-center">
        <div className="flex items-center justify-center space-x-2">
          <Sparkles className="h-8 w-8 text-primary" />
          <h1 className="text-3xl font-bold">Smart Templates Demo</h1>
        </div>
        <p className="mx-auto max-w-2xl text-lg text-muted-foreground">
          Discover the power of automated report generation with our
          comprehensive Smart Templates system. Follow this guided tour to
          explore all features and capabilities.
        </p>
      </div>

      {/* Progress */}
      <Card>
        <CardContent className="p-6">
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="font-medium">Demo Progress</h3>
              <Badge variant="outline">
                {completedSteps.length} of {demoSteps.length} completed
              </Badge>
            </div>
            <Progress value={progress} className="h-3" />
            <p className="text-sm text-muted-foreground">
              Complete all steps to become a Smart Templates expert!
            </p>
          </div>
        </CardContent>
      </Card>

      {/* Demo Steps */}
      <div className="grid gap-4">
        {demoSteps.map((step, index) => (
          <Card
            key={step.id}
            className={`cursor-pointer transition-all hover:shadow-md ${
              isStepCompleted(step.id)
                ? 'border-green-200 bg-green-50/50 dark:border-green-800 dark:bg-green-900/20'
                : index === currentStep
                  ? 'border-primary'
                  : ''
            }`}
            onClick={() => handleStepClick(step, index)}
          >
            <CardContent className="p-6">
              <div className="flex items-start space-x-4">
                <div
                  className={`rounded-lg p-3 ${
                    isStepCompleted(step.id)
                      ? 'bg-green-100 text-green-600 dark:bg-green-800 dark:text-green-200'
                      : 'bg-primary/10 text-primary'
                  }`}
                >
                  {isStepCompleted(step.id) ? (
                    <CheckCircle className="h-5 w-5" />
                  ) : (
                    step.icon
                  )}
                </div>

                <div className="flex-1 space-y-2">
                  <div className="flex items-center justify-between">
                    <h3 className="text-lg font-semibold">{step.title}</h3>
                    <div className="flex items-center space-x-2">
                      {isStepCompleted(step.id) && (
                        <Badge className="bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200">
                          Completed
                        </Badge>
                      )}
                      <Badge variant="outline">Step {index + 1}</Badge>
                    </div>
                  </div>

                  <p className="text-muted-foreground">{step.description}</p>

                  <div className="flex items-center justify-between pt-2">
                    <Button
                      variant={isStepCompleted(step.id) ? 'outline' : 'default'}
                      className="flex items-center space-x-2"
                    >
                      <span>{step.action}</span>
                      <ArrowRight className="h-4 w-4" />
                    </Button>

                    {index === currentStep && !isStepCompleted(step.id) && (
                      <Badge className="bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-200">
                        Current Step
                      </Badge>
                    )}
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Quick Actions */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center space-x-2">
            <Zap className="h-5 w-5" />
            <span>Quick Actions</span>
          </CardTitle>
          <CardDescription>
            Jump directly to key features or start fresh
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Button
              variant="outline"
              className="flex h-auto flex-col items-center space-y-2 p-4"
              onClick={() =>
                router.push('/plugins/smart-templates/templates/create')
              }
            >
              <Sparkles className="h-6 w-6" />
              <span className="text-sm">Create Template</span>
            </Button>

            <Button
              variant="outline"
              className="flex h-auto flex-col items-center space-y-2 p-4"
              onClick={() =>
                router.push('/plugins/smart-templates/reports/generate')
              }
            >
              <Play className="h-6 w-6" />
              <span className="text-sm">Generate Report</span>
            </Button>

            <Button
              variant="outline"
              className="flex h-auto flex-col items-center space-y-2 p-4"
              onClick={() => router.push('/plugins/smart-templates/analytics')}
            >
              <BarChart3 className="h-6 w-6" />
              <span className="text-sm">View Analytics</span>
            </Button>

            <Button
              variant="outline"
              className="flex h-auto flex-col items-center space-y-2 p-4"
              onClick={() => {
                setCompletedSteps([])
                setCurrentStep(0)
              }}
            >
              <Zap className="h-6 w-6" />
              <span className="text-sm">Reset Demo</span>
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Completion Message */}
      {completedSteps.length === demoSteps.length && (
        <Card className="border-green-200 bg-green-50/50 dark:border-green-800 dark:bg-green-900/20">
          <CardContent className="space-y-4 p-6 text-center">
            <div className="flex items-center justify-center space-x-2">
              <CheckCircle className="h-8 w-8 text-green-600" />
              <h3 className="text-xl font-bold text-green-800 dark:text-green-200">
                Demo Complete! 🎉
              </h3>
            </div>
            <p className="text-green-700 dark:text-green-300">
              Congratulations! you&apos;ve successfully explored all Smart
              Templates features. You&apos;re now ready to create powerful
              reports and templates for your organization.
            </p>
            <div className="flex items-center justify-center space-x-4">
              <Button
                onClick={() => router.push('/plugins/smart-templates')}
                className="flex items-center space-x-2"
              >
                <BarChart3 className="h-4 w-4" />
                <span>Go to Dashboard</span>
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  setCompletedSteps([])
                  setCurrentStep(0)
                }}
              >
                Restart Demo
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
