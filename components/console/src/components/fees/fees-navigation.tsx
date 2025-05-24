'use client'

import React from 'react'
import { usePathname, useRouter } from 'next/navigation'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Home, Package, Calculator, TrendingUp } from 'lucide-react'

export function FeesNavigation() {
  const pathname = usePathname()
  const router = useRouter()

  const navItems = [
    {
      label: 'Overview',
      href: '/plugins/fees',
      icon: Home
    },
    {
      label: 'Fee Packages',
      href: '/plugins/fees/packages',
      icon: Package
    },
    {
      label: 'Calculator',
      href: '/plugins/fees/calculator',
      icon: Calculator
    },
    {
      label: 'Analytics',
      href: '/plugins/fees/analytics',
      icon: TrendingUp
    }
  ]

  const isActive = (href: string) => {
    if (href === '/plugins/fees') {
      return pathname === href
    }
    return pathname.startsWith(href)
  }

  return (
    <nav className="flex space-x-1 border-b">
      {navItems.map((item) => {
        const Icon = item.icon
        const active = isActive(item.href)

        return (
          <Button
            key={item.href}
            variant="ghost"
            size="sm"
            className={cn(
              'flex items-center gap-2 rounded-none border-b-2 border-transparent px-4 py-2',
              'hover:border-gray-300 hover:bg-transparent',
              active && 'border-primary text-primary hover:border-primary'
            )}
            onClick={() => router.push(item.href)}
          >
            <Icon className="h-4 w-4" />
            {item.label}
          </Button>
        )
      })}
    </nav>
  )
}
