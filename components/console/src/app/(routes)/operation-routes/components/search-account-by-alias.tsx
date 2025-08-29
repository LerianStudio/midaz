import React, { useState } from 'react'
import { Controller, Control } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { useDebounce } from '@/hooks/use-debounce-value'
import { useListAccounts } from '@/client/accounts'
import { useOrganization } from '@lerianstudio/console-layout'
import { FormTooltip } from '@/components/ui/form'

export interface SearchAccountByAliasProps {
  control: Control<any>
  name: string
  label?: string
  tooltip?: string
  placeholder?: string
  required?: boolean
  disabled?: boolean
  rules?: any
  maxSuggestions?: number
  debounceDelay?: number
  className?: string
  onAccountSelect?: (account: AccountDto) => void
  onSearchChange?: (searchTerm: string) => void
}

export interface AccountDto {
  id: string
  ledgerId: string
  assetCode: string
  organizationId: string
  name: string
  alias: string
  type: string
  entityId: string
  parentAccountId: string
  portfolioId?: string | null
  segmentId: string
  allowSending?: boolean
  allowReceiving?: boolean
  metadata: any
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}

export const SearchAccountByAlias: React.FC<SearchAccountByAliasProps> = ({
  control,
  name,
  label,
  tooltip,
  placeholder,
  required = false,
  disabled = false,
  rules,
  maxSuggestions = 5,
  debounceDelay = 300,
  className = '',
  onAccountSelect,
  onSearchChange
}) => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()
  const [searchTerm, setSearchTerm] = useState('')
  const debouncedSearchTerm = useDebounce(searchTerm, debounceDelay)

  const { data: accountsData, isLoading: accountsLoading, error: accountsError } = useListAccounts({
    organizationId: currentOrganization?.id || '',
    ledgerId: currentLedger?.id || '',
    query: debouncedSearchTerm ? { alias: debouncedSearchTerm, limit: maxSuggestions + 10 } : undefined,
    enabled: !!currentOrganization?.id && !!currentLedger?.id && !!debouncedSearchTerm
  })

  const handleInputChange = (value: string, onChange: (value: string) => void) => {
    setSearchTerm(value)
    onChange(value)
    onSearchChange?.(value)
  }

  const handleAccountSelect = (account: AccountDto, onChange: (value: string) => void) => {
    onChange(account.alias)
    setSearchTerm('')
    onAccountSelect?.(account)
  }

  const defaultLabel = intl.formatMessage({
    id: 'operation-routes.field.validIf.alias',
    defaultMessage: 'Account Alias'
  })

  const defaultTooltip = intl.formatMessage({
    id: 'operation-routes.field.validIf.alias.tooltip',
    defaultMessage: 'Enter the account alias to validate against'
  })

  const defaultPlaceholder = intl.formatMessage({
    id: 'operation-routes.field.validIf.alias.placeholder',
    defaultMessage: 'Type to search account alias (e.g., @account123)'
  })

  return (
    <div className={`space-y-2 w-full ${className}`}>
      <div className='flex justify-between w-full'>
        <Label className="text-sm font-medium">
          {label || defaultLabel}
          {required && <span className="ml-1">*</span>}
        </Label>
        <FormTooltip>
          {tooltip || defaultTooltip}
        </FormTooltip>
      </div>
      <Controller
        name={name}
        control={control}
        rules={{ required, ...rules }}
        render={({ field, fieldState }) => (
          <div className="space-y-1">
            <div className="space-y-2">
              <Input
                {...field}
                value={field.value || ''}
                onChange={(e) => handleInputChange(e.target.value, field.onChange)}
                placeholder={placeholder || defaultPlaceholder}
                disabled={disabled}
                className={`w-full ${fieldState.error ? 'border-red-500' : ''}`}
              />
              {debouncedSearchTerm && accountsData?.items && accountsData.items.length > 0 && (
                <div className="w-full max-h-32 overflow-y-auto border rounded-md p-2 space-y-1 bg-muted/50">
                  <p className="text-xs text-muted-foreground mb-2">
                    {intl.formatMessage({
                      id: 'search-account-by-alias.available-aliases',
                      defaultMessage: 'Available aliases:'
                    })}
                  </p>
                  {accountsData.items.slice(0, maxSuggestions).map((account) => (
                    <div
                      key={account.id}
                      className="text-sm p-1 hover:bg-muted cursor-pointer rounded transition-colors"
                      onClick={() => handleAccountSelect(account, field.onChange)}
                    >
                      <span className="font-medium">{account.alias}</span>
                      <span className="text-muted-foreground ml-2">- {account.name}</span>
                    </div>
                  ))}
                  {accountsData.items.length > maxSuggestions && (
                    <p className="text-xs text-muted-foreground">
                      {intl.formatMessage({
                        id: 'search-account-by-alias.more-results',
                        defaultMessage: '... and {count} more'
                      }, { count: accountsData.items.length - maxSuggestions })}
                    </p>
                  )}
                </div>
              )}
              {debouncedSearchTerm && accountsLoading && (
                <p className="text-xs text-muted-foreground">
                  {intl.formatMessage({
                    id: 'search-account-by-alias.searching',
                    defaultMessage: 'Searching...'
                  })}
                </p>
              )}
              {debouncedSearchTerm && !accountsLoading && (!accountsData?.items || accountsData.items.length === 0) && (
                <p className="text-xs text-muted-foreground">
                  {intl.formatMessage({
                    id: 'search-account-by-alias.no-aliases-found',
                    defaultMessage: 'No aliases found'
                  })}
                </p>
              )}
              {accountsError && (
                <p className="text-xs text-red-500">
                  {intl.formatMessage({
                    id: 'search-account-by-alias.error-loading',
                    defaultMessage: 'Error loading accounts'
                  })}
                </p>
              )}
            </div>
            {fieldState.error && (
              <p className="text-sm text-red-500">
                {fieldState.error.message}
              </p>
            )}
          </div>
        )}
      />
    </div>
  )
}

// Example usage:
//
// Basic usage with default settings:
// <SearchAccountByAlias
//   control={form.control}
//   name="accountAlias"
//   required
// />
//
// Advanced usage with customizations:
// <SearchAccountByAlias
//   control={form.control}
//   name="selectedAccount"
//   label="Select Account"
//   tooltip="Choose an account to associate with this transaction"
//   placeholder="Start typing to find accounts..."
//   required
//   maxSuggestions={10}
//   debounceDelay={500}
//   onAccountSelect={(account) => console.log('Selected account:', account)}
//   onSearchChange={(term) => console.log('Search term changed:', term)}
// />
//
// Usage in a form with custom validation:
// <SearchAccountByAlias
//   control={form.control}
//   name="accountAlias"
//   required
//   rules={{
//     validate: (value) => value?.startsWith('@') || 'Alias must start with @'
//   }}
//   className="custom-styling"
// />