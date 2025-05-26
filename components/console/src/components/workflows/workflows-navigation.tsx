'use client'

import { usePathname } from 'next/navigation'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import {
  GitBranch,
  Play,
  Layers,
  Plug,
  BarChart3,
  FileText,
  Settings,
  Zap
} from 'lucide-react'

interface NavItem {
  title: string
  href: string
  icon: any
  description: string
  highlight?: boolean
}

const workflowsNavItems: NavItem[] = [
  {
    title: 'Overview',
    href: '/plugins/workflows',
    icon: GitBranch,
    description: 'Workflow dashboard and metrics'
  },
  {
    title: 'Library',
    href: '/plugins/workflows/library',
    icon: Layers,
    description: 'Workflow definitions and templates'
  },
  {
    title: 'Executions',
    href: '/plugins/workflows/executions',
    icon: Play,
    description: 'Monitor and manage workflow executions'
  },
  {
    title: 'Tasks',
    href: '/plugins/workflows/tasks',
    icon: FileText,
    description: 'Task definitions and testing'
  },
  {
    title: 'Integrations',
    href: '/plugins/workflows/integrations',
    icon: Plug,
    description: 'Service integrations and endpoints'
  },
  {
    title: 'Analytics',
    href: '/plugins/workflows/analytics',
    icon: BarChart3,
    description: 'Performance metrics and insights'
  },
  {
    title: 'Demos',
    href: '/plugins/workflows/demo',
    icon: Zap,
    description: 'Interactive workflow demos and examples',
    highlight: true
  }
]

export function WorkflowsNavigation() {
  const pathname = usePathname()

  return (
    <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container mx-auto px-4">
        <nav className="scrollbar-hide flex items-center space-x-1 overflow-x-auto py-2">
          {workflowsNavItems.map((item) => {
            const isActive =
              pathname === item.href ||
              (item.href !== '/plugins/workflows' &&
                pathname.startsWith(item.href))

            return (
              <Link
                key={item.href}
                href={item.href}
                className={cn(
                  'relative flex items-center space-x-2 whitespace-nowrap rounded-lg px-4 py-2 text-sm font-medium transition-all duration-200 hover:bg-accent hover:text-accent-foreground',
                  isActive
                    ? 'bg-primary text-primary-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground',
                  item.highlight &&
                    !isActive &&
                    'bg-primary/10 text-primary hover:bg-primary/20'
                )}
                title={item.description}
              >
                <item.icon
                  className={cn(
                    'h-4 w-4',
                    item.highlight && !isActive && 'text-primary'
                  )}
                />
                <span>{item.title}</span>
                {isActive && (
                  <div className="h-1 w-1 rounded-full bg-primary-foreground/60" />
                )}
                {item.highlight && !isActive && (
                  <span className="absolute -right-1 -top-1 flex h-3 w-3">
                    <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75"></span>
                    <span className="relative inline-flex h-3 w-3 rounded-full bg-primary"></span>
                  </span>
                )}
              </Link>
            )
          })}
        </nav>
      </div>
    </div>
  )
}
