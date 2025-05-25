'use client'

import { usePathname } from 'next/navigation'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import {
  FileText,
  Activity,
  GitMerge,
  AlertTriangle,
  Settings,
  Database,
  BarChart3,
  Clock,
  CheckCircle,
  FileX
} from 'lucide-react'

interface ReconciliationNavigationProps {
  basePath?: string
}

const navigationItems = [
  {
    name: 'Overview',
    href: '',
    icon: BarChart3,
    description: 'Reconciliation dashboard and metrics'
  },
  {
    name: 'Imports',
    href: '/imports',
    icon: FileText,
    description: 'Transaction file imports and processing',
    badge: 'pending'
  },
  {
    name: 'Processes',
    href: '/processes',
    icon: Activity,
    description: 'Reconciliation process monitoring',
    badge: 'active'
  },
  {
    name: 'Matches',
    href: '/matches',
    icon: GitMerge,
    description: 'Transaction matches and reviews',
    badge: 'review'
  },
  {
    name: 'Exceptions',
    href: '/exceptions',
    icon: AlertTriangle,
    description: 'Exception management and resolution',
    badge: 'critical'
  },
  {
    name: 'Rules',
    href: '/rules',
    icon: Settings,
    description: 'Matching rules and configuration'
  },
  {
    name: 'Sources',
    href: '/sources',
    icon: Database,
    description: 'Data sources and orchestration'
  },
  {
    name: 'Analytics',
    href: '/analytics',
    icon: BarChart3,
    description: 'Analytics and reporting'
  }
]

const getBadgeInfo = (badge?: string) => {
  switch (badge) {
    case 'pending':
      return {
        icon: Clock,
        color:
          'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400'
      }
    case 'active':
      return {
        icon: Activity,
        color:
          'bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400'
      }
    case 'review':
      return {
        icon: CheckCircle,
        color:
          'bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400'
      }
    case 'critical':
      return {
        icon: FileX,
        color: 'bg-red-100 text-red-800 dark:bg-red-900/20 dark:text-red-400'
      }
    default:
      return null
  }
}

export function ReconciliationNavigation({
  basePath = '/plugins/reconciliation'
}: ReconciliationNavigationProps) {
  const pathname = usePathname()

  return (
    <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container mx-auto px-4">
        <nav className="scrollbar-hide flex items-center space-x-1 overflow-x-auto py-2">
          {navigationItems.map((item) => {
            const href = `${basePath}${item.href}`
            const isActive =
              pathname === href ||
              (item.href !== '' && pathname.startsWith(href))
            const Icon = item.icon
            const badgeInfo = getBadgeInfo(item.badge)

            return (
              <Link
                key={item.name}
                href={href}
                className={cn(
                  'flex items-center space-x-2 whitespace-nowrap rounded-lg px-4 py-2 text-sm font-medium transition-all duration-200 hover:bg-accent hover:text-accent-foreground',
                  isActive
                    ? 'bg-primary text-primary-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground'
                )}
                title={item.description}
              >
                <Icon className="h-4 w-4" />
                <span>{item.name}</span>
                {badgeInfo && (
                  <div
                    className={cn(
                      'flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium',
                      badgeInfo.color
                    )}
                  >
                    <badgeInfo.icon className="h-3 w-3" />
                    <span className="hidden sm:inline">{item.badge}</span>
                  </div>
                )}
                {isActive && !badgeInfo && (
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

export default ReconciliationNavigation
