'use client'

import { Database, External, Info } from 'lucide-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'

interface DomainSelectorProps {
  value: 'ledger' | 'external' | null
  onChange: (domain: 'ledger' | 'external') => void
  disabled?: boolean
}

export function DomainSelector({
  value,
  onChange,
  disabled = false
}: DomainSelectorProps) {
  const domains = [
    {
      id: 'ledger' as const,
      title: 'Ledger Domain',
      description: 'Internal accounts managed within the Midaz ledger system',
      icon: Database,
      color: 'bg-blue-50 border-blue-200 hover:bg-blue-100',
      iconColor: 'text-blue-600',
      badgeColor: 'bg-blue-100 text-blue-700',
      features: [
        'Full transaction history tracking',
        'Real-time balance calculations',
        'Integrated compliance validation',
        'Native audit trail support'
      ]
    },
    {
      id: 'external' as const,
      title: 'External Domain',
      description: 'External system accounts for third-party integrations',
      icon: External,
      color: 'bg-orange-50 border-orange-200 hover:bg-orange-100',
      iconColor: 'text-orange-600',
      badgeColor: 'bg-orange-100 text-orange-700',
      features: [
        'External system integration',
        'Wire transfer support',
        'Third-party validation',
        'Settlement processing'
      ]
    }
  ]

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        {domains.map((domain) => {
          const Icon = domain.icon
          const isSelected = value === domain.id

          return (
            <Card
              key={domain.id}
              className={`cursor-pointer transition-all ${
                isSelected
                  ? `${domain.color} ring-2 ring-blue-500 ring-offset-2`
                  : `hover:${domain.color}`
              } ${disabled ? 'cursor-not-allowed opacity-50' : ''}`}
              onClick={() => !disabled && onChange(domain.id)}
            >
              <CardHeader className="pb-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Icon className={`h-5 w-5 ${domain.iconColor}`} />
                    <CardTitle className="text-lg">{domain.title}</CardTitle>
                  </div>
                  {isSelected && (
                    <Badge className={domain.badgeColor}>Selected</Badge>
                  )}
                </div>
                <CardDescription className="text-sm">
                  {domain.description}
                </CardDescription>
              </CardHeader>
              <CardContent className="pt-0">
                <ul className="space-y-1">
                  {domain.features.map((feature, index) => (
                    <li
                      key={index}
                      className="flex items-center gap-2 text-sm text-gray-600"
                    >
                      <div className="h-1.5 w-1.5 flex-shrink-0 rounded-full bg-gray-400" />
                      {feature}
                    </li>
                  ))}
                </ul>
              </CardContent>
            </Card>
          )
        })}
      </div>

      {value && (
        <Alert>
          <Info className="h-4 w-4" />
          <AlertDescription>
            {value === 'ledger'
              ? 'Ledger domain accounts are managed internally with full transaction tracking and real-time validation. They support all Midaz accounting features including automated compliance checks.'
              : 'External domain accounts represent third-party systems and external bank accounts. They support wire transfers, settlement processing, and integration with external financial institutions.'}
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}
