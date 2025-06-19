import { useListAccounts } from '@/client/accounts'
import { Button } from '@/components/ui/button'
import { Form, FormField, FormItem, FormMessage } from '@/components/ui/form'
import {
  Autocomplete,
  AutocompleteContent,
  AutocompleteEmpty,
  AutocompleteGroup,
  AutocompleteItem,
  AutocompleteLoading,
  AutocompleteTrigger,
  AutocompleteValue
} from '@/components/ui/autocomplete'
import { Paper } from '@/components/ui/paper'
import { useOrganization } from '@/providers/organization-provider'
import { zodResolver } from '@hookform/resolvers/zod'
import { Plus } from 'lucide-react'
import React, { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useDebounce } from '@/hooks/use-debounce'
import { AccountDto } from '@/core/application/dto/account-dto'
import { useIntl } from 'react-intl'
import { cn } from '@/lib/utils'
import { CustomFormErrors } from '@/hooks/use-custom-form-error'

const initialValues = {
  search: ''
}

const SearchSchema = z.object({
  search: z.string().max(255)
})

type AccountSearchFieldProps = {
  className?: string
  errors?: CustomFormErrors
  onSelect?: (accountId: string, account: AccountDto) => void
  onClear?: () => void
}

export const AccountSearchField = ({
  className,
  errors,
  onSelect,
  onClear
}: AccountSearchFieldProps) => {
  const intl = useIntl()
  const [selected, setSelected] = useState('')
  const [query, setQuery] = useState('')
  const { currentOrganization, currentLedger } = useOrganization()

  const form = useForm({
    errors,
    resolver: zodResolver(SearchSchema),
    defaultValues: initialValues,
    mode: 'onChange'
  })
  const search = form.watch('search')

  const { data: accounts, isFetching: loading } = useListAccounts({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id!,
    query: {
      alias: query
    },
    enabled: query !== ''
  })

  useDebounce(
    async () => {
      // First validate the field
      const result = await form.trigger()
      if (!result) {
        return
      }

      setQuery(search)
    },
    500,
    [search.toString()]
  )

  const handleSubmit = React.useCallback(() => {
    const account = accounts?.items?.find(
      (account) => account.alias === selected
    )

    onSelect?.(account?.alias!, account!)
    setQuery('')
    setSelected('')
    form.reset()
  }, [form, selected, accounts, onSelect])

  return (
    <Paper className={cn('w-full px-6 py-5', className)}>
      <Form {...form}>
        <form
          className="flex w-full flex-row gap-3"
          noValidate
          onSubmit={handleSubmit}
        >
          <FormField
            name="search"
            render={({ field: { ref, value, onChange, ...fieldOthers } }) => (
              <FormItem ref={ref} className="w-full">
                <Autocomplete
                  className="w-full"
                  value={selected}
                  onValueChange={(value) => setSelected(value as string)}
                  onClear={onClear}
                >
                  <AutocompleteTrigger>
                    <AutocompleteValue
                      placeholder={intl.formatMessage({
                        id: 'transactions.create.searchByAlias',
                        defaultMessage: 'Search by alias'
                      })}
                      value={value}
                      onValueChange={onChange}
                      {...fieldOthers}
                    />
                  </AutocompleteTrigger>
                  <AutocompleteContent>
                    {loading && <AutocompleteLoading />}

                    {!loading && (
                      <AutocompleteEmpty className="text-zinc-500">
                        {intl.formatMessage({
                          id: 'common.noOptions',
                          defaultMessage: 'No options found.'
                        })}
                      </AutocompleteEmpty>
                    )}

                    <AutocompleteGroup>
                      {!loading &&
                        search !== '' &&
                        accounts?.items?.map?.((account) => (
                          <AutocompleteItem
                            key={account.id}
                            value={account.alias!}
                          >
                            {account.alias}
                          </AutocompleteItem>
                        ))}
                    </AutocompleteGroup>
                  </AutocompleteContent>
                </Autocomplete>
                <FormMessage />
              </FormItem>
            )}
          />

          <Button
            className="bg-shadcn-600 disabled:bg-shadcn-200 h-9 w-9 self-end rounded-full"
            onClick={handleSubmit}
            disabled={loading || !selected}
          >
            <Plus className="h-4 w-4 shrink-0" />
          </Button>
        </form>
      </Form>
    </Paper>
  )
}
