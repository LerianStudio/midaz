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

const workflowsNavItems = [
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
                  'flex items-center space-x-2 whitespace-nowrap rounded-lg px-4 py-2 text-sm font-medium transition-all duration-200 hover:bg-accent hover:text-accent-foreground',
                  isActive
                    ? 'bg-primary text-primary-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground'
                )}
                title={item.description}
              >
                <item.icon className="h-4 w-4" />
                <span>{item.title}</span>
                {isActive && (
                  <div className="h-1 w-1 rounded-full bg-primary-foreground/60" />
                )}
              </Link>
            )
          })}
        </nav>
      </div>
    </div>
  )
}
