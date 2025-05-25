'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { usePathname } from 'next/navigation'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import { Users, BarChart3, CreditCard, TrendingUp } from 'lucide-react'

export const CRMNavigation: React.FC = () => {
  const intl = useIntl()
  const pathname = usePathname()

  const navigationItems = [
    {
      id: 'overview',
      label: intl.formatMessage({
        id: 'crm.nav.overview',
        defaultMessage: 'Overview'
      }),
      href: '/plugins/crm',
      icon: <BarChart3 className="h-4 w-4" />,
      description: intl.formatMessage({
        id: 'crm.nav.overview.description',
        defaultMessage: 'CRM dashboard and key metrics'
      })
    },
    {
      id: 'customers',
      label: intl.formatMessage({
        id: 'crm.nav.customers',
        defaultMessage: 'Customers'
      }),
      href: '/plugins/crm/customers',
      icon: <Users className="h-4 w-4" />,
      description: intl.formatMessage({
        id: 'crm.nav.customers.description',
        defaultMessage: 'Manage individual and corporate customer profiles'
      })
    },
    {
      id: 'aliases',
      label: intl.formatMessage({
        id: 'crm.nav.aliases',
        defaultMessage: 'Banking Aliases'
      }),
      href: '/plugins/crm/aliases',
      icon: <CreditCard className="h-4 w-4" />,
      description: intl.formatMessage({
        id: 'crm.nav.aliases.description',
        defaultMessage: 'Customer-to-account relationship management'
      })
    },
    {
      id: 'analytics',
      label: intl.formatMessage({
        id: 'crm.nav.analytics',
        defaultMessage: 'Analytics'
      }),
      href: '/plugins/crm/analytics',
      icon: <TrendingUp className="h-4 w-4" />,
      description: intl.formatMessage({
        id: 'crm.nav.analytics.description',
        defaultMessage: 'Customer insights and reporting'
      })
    }
  ]

  return (
    <div className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container mx-auto px-4">
        <nav className="scrollbar-hide flex items-center space-x-1 overflow-x-auto py-2">
          {navigationItems.map((item) => {
            const isActive =
              pathname === item.href ||
              (item.href !== '/plugins/crm' && pathname.startsWith(item.href))

            return (
              <Link
                key={item.id}
                href={item.disabled ? '#' : item.href}
                className={cn(
                  'flex items-center space-x-2 whitespace-nowrap rounded-lg px-4 py-2 text-sm font-medium transition-all duration-200 hover:bg-accent hover:text-accent-foreground',
                  isActive
                    ? 'bg-primary text-primary-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground',
                  item.disabled && 'cursor-not-allowed opacity-50'
                )}
                title={item.description}
                onClick={(e: React.MouseEvent) =>
                  item.disabled && e.preventDefault()
                }
              >
                {item.icon}
                <span>{item.label}</span>
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
