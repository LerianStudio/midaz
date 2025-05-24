'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { usePathname } from 'next/navigation'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import { Users, BarChart3, CreditCard, TrendingUp } from 'lucide-react'

interface CRMNavigationProps {
  className?: string
}

export const CRMNavigation: React.FC<CRMNavigationProps> = ({ className }) => {
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
      }),
      disabled: true
    }
  ]

  return (
    <div className={cn('border-b bg-background', className)}>
      <div className="flex h-10 items-center space-x-8 px-6">
        {navigationItems.map((item) => {
          const isActive =
            pathname === item.href || pathname.startsWith(item.href + '/')

          return (
            <Link
              key={item.id}
              href={item.disabled ? '#' : item.href}
              className={cn(
                'flex items-center space-x-2 border-b-2 border-transparent px-1 py-2 text-sm font-medium transition-colors hover:text-foreground',
                isActive
                  ? 'border-primary text-foreground'
                  : 'text-muted-foreground',
                item.disabled && 'cursor-not-allowed opacity-50'
              )}
              onClick={(e: React.MouseEvent) =>
                item.disabled && e.preventDefault()
              }
            >
              {item.icon}
              <span>{item.label}</span>
            </Link>
          )
        })}
      </div>
    </div>
  )
}
