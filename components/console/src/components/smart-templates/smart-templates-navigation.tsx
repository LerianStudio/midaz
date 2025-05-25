'use client'

import React from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { cn } from '@/lib/utils'
import { FileText, Activity, Database, BarChart3 } from 'lucide-react'

const navigationItems = [
  {
    name: 'Overview',
    href: '/plugins/smart-templates',
    icon: Activity,
    description: 'Dashboard and overview'
  },
  {
    name: 'Templates',
    href: '/plugins/smart-templates/templates',
    icon: FileText,
    description: 'Template library and management'
  },
  {
    name: 'Reports',
    href: '/plugins/smart-templates/reports',
    icon: BarChart3,
    description: 'Generated reports and history'
  },
  {
    name: 'Data Sources',
    href: '/plugins/smart-templates/data-sources',
    icon: Database,
    description: 'Data source connections'
  },
  {
    name: 'Analytics',
    href: '/plugins/smart-templates/analytics',
    icon: BarChart3,
    description: 'Usage and performance metrics'
  }
]

export function SmartTemplatesNavigation() {
  const pathname = usePathname()

  return (
    <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container mx-auto px-4">
        <nav className="scrollbar-hide flex items-center space-x-1 overflow-x-auto py-2">
          {navigationItems.map((item) => {
            const isActive =
              pathname === item.href ||
              (item.href !== '/plugins/smart-templates' &&
                pathname?.startsWith(item.href))

            return (
              <Link
                key={item.name}
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
                <span>{item.name}</span>
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
