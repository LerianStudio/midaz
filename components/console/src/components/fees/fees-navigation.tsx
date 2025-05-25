'use client'

import React from 'react'
import { usePathname } from 'next/navigation'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import { Home, Package, Calculator, TrendingUp } from 'lucide-react'

const feesNavItems = [
  {
    title: 'Overview',
    href: '/plugins/fees',
    icon: Home,
    description: 'Fee management dashboard and metrics'
  },
  {
    title: 'Fee Packages',
    href: '/plugins/fees/packages',
    icon: Package,
    description: 'Create and manage fee packages'
  },
  {
    title: 'Calculator',
    href: '/plugins/fees/calculator',
    icon: Calculator,
    description: 'Fee calculation tools and testing'
  },
  {
    title: 'Analytics',
    href: '/plugins/fees/analytics',
    icon: TrendingUp,
    description: 'Fee analytics and reporting'
  }
]

export function FeesNavigation() {
  const pathname = usePathname()

  return (
    <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container mx-auto px-4">
        <nav className="scrollbar-hide flex items-center space-x-1 overflow-x-auto py-2">
          {feesNavItems.map((item) => {
            const isActive =
              pathname === item.href ||
              (item.href !== '/plugins/fees' &&
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
