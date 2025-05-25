'use client'

import { usePathname } from 'next/navigation'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import {
  BarChart3,
  Building2,
  GitBranch,
  Route,
  Shield,
  TrendingUp
} from 'lucide-react'

const accountingNavItems = [
  {
    title: 'Overview',
    href: '/plugins/accounting',
    icon: BarChart3,
    description: 'Accounting dashboard and metrics'
  },
  {
    title: 'Account Types',
    href: '/plugins/accounting/account-types',
    icon: Building2,
    description: 'Chart of accounts management'
  },
  {
    title: 'Transaction Routes',
    href: '/plugins/accounting/transaction-routes',
    icon: GitBranch,
    description: 'Transaction template management'
  },
  {
    title: 'Operation Routes',
    href: '/plugins/accounting/operation-routes',
    icon: Route,
    description: 'Operation mapping and configuration'
  },
  {
    title: 'Compliance',
    href: '/plugins/accounting/compliance',
    icon: Shield,
    description: 'Compliance and validation monitoring'
  },
  {
    title: 'Analytics',
    href: '/plugins/accounting/analytics',
    icon: TrendingUp,
    description: 'Insights and reporting'
  }
]

export function AccountingNavigation() {
  const pathname = usePathname()

  return (
    <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container mx-auto px-4">
        <nav className="scrollbar-hide flex items-center space-x-1 overflow-x-auto py-2">
          {accountingNavItems.map((item) => {
            const isActive =
              pathname === item.href ||
              (item.href !== '/plugins/accounting' &&
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
