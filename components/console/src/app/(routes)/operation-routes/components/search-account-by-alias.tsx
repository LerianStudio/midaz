import React, { useState, useCallback } from 'react'
import { Controller, Control } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Loader2 } from 'lucide-react'
import { useDebounce } from '@/hooks/use-debounce'
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
  const [debouncedSearchTerm, setDebouncedSearchTerm] = useState('')

  // Debounced callback that updates the search term for API calls
  const debouncedSearchCallback = useCallback(() => {
    setDebouncedSearchTerm(searchTerm)
  }, [searchTerm])

  // Use the callback-based debounce hook
  useDebounce(debouncedSearchCallback, debounceDelay, [searchTerm])

  const { data: accountsData, isLoading: accountsLoading, error: accountsError } = useListAccounts({
    organizationId: currentOrganization?.id || '',
    ledgerId: currentLedger?.id || '',
    query: debouncedSearchTerm ? { alias: debouncedSearchTerm } : undefined,
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
    setDebouncedSearchTerm('')
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
            <Popover>
              <PopoverTrigger asChild>
                <div className="relative">
                  <Input
                    {...field}
                    value={field.value || ''}
                    onChange={(e) => handleInputChange(e.target.value, field.onChange)}
                    placeholder={placeholder || defaultPlaceholder}
                    disabled={disabled}
                    className={`w-full pr-10 ${fieldState.error ? 'border-red-500' : ''}`}
                  />
                  {/* Loading Indicator */}
                  {accountsLoading && (
                    <div className="absolute right-3 top-1/2 -translate-y-1/2">
                      <Loader2 className="h-4 w-4 animate-spin text-primary" />
                    </div>
                  )}
                </div>
              </PopoverTrigger>

              {/* Show dropdown only when searching or has results */}
              {(debouncedSearchTerm || accountsLoading) && (
                <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0" align="start">
                  <div className="max-h-64 overflow-y-auto">
                    {/* Loading State */}
                    {accountsLoading && (
                      <div className="p-4 flex items-center gap-2">
                        <Loader2 className="h-4 w-4 animate-spin text-primary" />
                        <p className="text-sm text-muted-foreground">
                          {intl.formatMessage({
                            id: 'search-account-by-alias.searching',
                            defaultMessage: 'Searching...'
                          })}
                        </p>
                      </div>
                    )}

                    {/* Results */}
                    {!accountsLoading && accountsData?.items && accountsData.items.length > 0 && (
                      <div className="p-2">
                        <div className="space-y-1">
                          {accountsData.items.slice(0, maxSuggestions).map((account) => (
                            <div
                              key={account.id}
                              className="flex items-center p-2 hover:bg-muted cursor-pointer rounded-md transition-colors"
                              onClick={() => handleAccountSelect(account, field.onChange)}
                            >
                              <div className="flex-1">
                                <span className="font-medium text-sm">{account.alias}</span>
                                <span className="text-muted-foreground text-xs ml-2">- {account.name}</span>
                              </div>
                            </div>
                          ))}
                        </div>

                        {accountsData.items.length > maxSuggestions && (
                          <p className="text-xs text-muted-foreground text-center mt-2 px-2">
                            {intl.formatMessage({
                              id: 'search-account-by-alias.more-results',
                              defaultMessage: '... and {count} more'
                            }, { count: accountsData.items.length - maxSuggestions })}
                          </p>
                        )}
                      </div>
                    )}

                    {/* No Results */}
                    {!accountsLoading && debouncedSearchTerm && (!accountsData?.items || accountsData.items.length === 0) && (
                      <div className="p-4 text-center">
                        <p className="text-sm text-muted-foreground">
                          {intl.formatMessage({
                            id: 'search-account-by-alias.no-aliases-found',
                            defaultMessage: 'No aliases found'
                          })}
                        </p>
                      </div>
                    )}

                    {/* Error State */}
                    {accountsError && (
                      <div className="p-4 text-center">
                        <p className="text-sm text-red-500">
                          {intl.formatMessage({
                            id: 'search-account-by-alias.error-loading',
                            defaultMessage: 'Error loading accounts'
                          })}
                        </p>
                      </div>
                    )}
                  </div>
                </PopoverContent>
              )}
            </Popover>

            {fieldState.error && (
              <p className="text-sm text-red-500 mt-1">
                {fieldState.error.message}
              </p>
            )}
          </div>
        )}
      />
    </div>
  )
}
