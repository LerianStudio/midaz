import React, { useState, useCallback } from 'react'
import { Controller, Control } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger
} from '@/components/ui/popover'
import { Loader2 } from 'lucide-react'
import { useDebounce } from '@/hooks/use-debounce'
import { useListAccounts } from '@/client/accounts'
import { useOrganization } from '@lerianstudio/console-layout'
import { FormTooltip } from '@/components/ui/form'
import { AccountDto } from '@/core/application/dto/account-dto'

export interface SearchAccountByAliasFieldProps {
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
  'data-testid'?: string
}

export const SearchAccountByAliasField: React.FC<
  SearchAccountByAliasFieldProps
> = ({
  control,
  name,
  label,
  tooltip,
  placeholder,
  required = false,
  disabled = false,
  rules,
  maxSuggestions = 5,
  debounceDelay = 600,
  className = '',
  onAccountSelect,
  onSearchChange,
  'data-testid': dataTestId
}) => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()
  const [searchTerm, setSearchTerm] = useState('')
  const [debouncedSearchTerm, setDebouncedSearchTerm] = useState('')

  const debouncedSearchCallback = useCallback(() => {
    setDebouncedSearchTerm(searchTerm)
  }, [searchTerm])

  useDebounce(debouncedSearchCallback, debounceDelay, [searchTerm])

  const {
    data: accountsData,
    isLoading: accountsLoading,
    error: accountsError
  } = useListAccounts({
    organizationId: currentOrganization?.id || '',
    ledgerId: currentLedger?.id || '',
    query: debouncedSearchTerm ? { alias: debouncedSearchTerm } : undefined,
    enabled:
      !!currentOrganization?.id && !!currentLedger?.id && !!debouncedSearchTerm
  })

  const handleInputChange = (
    value: string,
    onChange: (value: string) => void
  ) => {
    setSearchTerm(value)
    onChange(value)
    onSearchChange?.(value)
  }

  const handleAccountSelect = (
    account: AccountDto,
    onChange: (value: string) => void
  ) => {
    onChange(account.alias)
    setSearchTerm('')
    setDebouncedSearchTerm('')
    onAccountSelect?.(account)
  }

  const defaultLabel = intl.formatMessage({
    id: 'searchAccountByAlias.label',
    defaultMessage: 'Account Alias'
  })

  const defaultTooltip = intl.formatMessage({
    id: 'searchAccountByAlias.tooltip',
    defaultMessage: 'Search and select an account by its alias'
  })

  const defaultPlaceholder = intl.formatMessage({
    id: 'searchAccountByAlias.placeholder',
    defaultMessage: 'Type to search account alias (e.g., @account123)'
  })

  return (
    <div className={`w-full space-y-2 ${className}`}>
      <div className="flex w-full justify-between">
        <Label className="text-sm font-medium">
          {label || defaultLabel}
          {required && <span className="ml-1">*</span>}
        </Label>
        <FormTooltip>{tooltip || defaultTooltip}</FormTooltip>
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
                    onChange={(e) =>
                      handleInputChange(e.target.value, field.onChange)
                    }
                    placeholder={placeholder || defaultPlaceholder}
                    disabled={disabled}
                    className={`w-full pr-10 ${fieldState.error ? 'border-red-500' : ''}`}
                    data-testid={dataTestId}
                  />
                  {/* Loading Indicator */}
                  {accountsLoading && (
                    <div className="absolute top-1/2 right-3 -translate-y-1/2">
                      <Loader2 className="text-primary h-4 w-4 animate-spin" />
                    </div>
                  )}
                </div>
              </PopoverTrigger>

              {/* Show dropdown only when searching or has results */}
              {(debouncedSearchTerm || accountsLoading) && (
                <PopoverContent
                  className="w-[var(--radix-popover-trigger-width)] p-0"
                  align="start"
                >
                  <div className="max-h-64 overflow-y-auto">
                    {/* Loading State */}
                    {accountsLoading && (
                      <div className="flex items-center gap-2 p-4">
                        <Loader2 className="text-primary h-4 w-4 animate-spin" />
                        <p className="text-muted-foreground text-sm">
                          {intl.formatMessage({
                            id: 'searchAccountByAlias.searching',
                            defaultMessage: 'Searching...'
                          })}
                        </p>
                      </div>
                    )}

                    {/* Results */}
                    {!accountsLoading &&
                      accountsData?.items &&
                      accountsData.items.length > 0 && (
                        <div className="p-2">
                          <div className="space-y-1">
                            {accountsData.items
                              .slice(0, maxSuggestions)
                              .map((account) => (
                                <div
                                  key={account.id}
                                  className="hover:bg-muted flex cursor-pointer items-center rounded-md p-2 transition-colors"
                                  onClick={() =>
                                    handleAccountSelect(account, field.onChange)
                                  }
                                >
                                  <div className="flex-1">
                                    <span className="text-sm font-medium">
                                      {account.alias}
                                    </span>
                                    <span className="text-muted-foreground ml-2 text-xs">
                                      - {account.name}
                                    </span>
                                  </div>
                                </div>
                              ))}
                          </div>

                          {accountsData.items.length > maxSuggestions && (
                            <p className="text-muted-foreground mt-2 px-2 text-center text-xs">
                              {intl.formatMessage(
                                {
                                  id: 'searchAccountByAlias.moreResults',
                                  defaultMessage: '... and {count} more'
                                },
                                {
                                  count:
                                    accountsData.items.length - maxSuggestions
                                }
                              )}
                            </p>
                          )}
                        </div>
                      )}

                    {/* No Results */}
                    {!accountsLoading &&
                      debouncedSearchTerm &&
                      (!accountsData?.items ||
                        accountsData.items.length === 0) && (
                        <div className="p-4 text-center">
                          <p className="text-muted-foreground text-sm">
                            {intl.formatMessage({
                              id: 'searchAccountByAlias.noAliasesFound',
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
                            id: 'searchAccountByAlias.errorLoading',
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
              <p className="mt-1 text-sm text-red-500">
                {fieldState.error.message}
              </p>
            )}
          </div>
        )}
      />
    </div>
  )
}
