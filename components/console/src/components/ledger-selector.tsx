'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { ChevronsUpDown, Database } from 'lucide-react'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectLabel,
  SelectItem
} from '@/components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem
} from '@/components/ui/command'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { useListLedgers } from '@/client/ledgers'
import { Button } from './ui/button'
import { LedgerResponseDto } from '@/core/application/dto/ledger-dto'

const LedgerCommand = ({
  ledgers,
  onSelect
}: {
  ledgers: LedgerResponseDto[]
  onSelect: (id: string) => void
}) => {
  const intl = useIntl()
  const [query, setQuery] = React.useState('')
  const [visibleCount, setVisibleCount] = React.useState(10)

  const filteredLedgers = React.useMemo(() => {
    if (!query) return ledgers
    return ledgers.filter((ledger) =>
      ledger.name.toLowerCase().includes(query.toLowerCase())
    )
  }, [ledgers, query])

  const displayedLedgers = query
    ? filteredLedgers
    : filteredLedgers.slice(0, visibleCount)

  const hasMore = !query && displayedLedgers.length < filteredLedgers.length

  const loadMore = () => {
    setVisibleCount((prev) => prev + 10)
  }

  return (
    <Command className="w-full">
      <CommandInput
        placeholder={intl.formatMessage({
          id: 'common.search',
          defaultMessage: 'Search...'
        })}
        value={query}
        onValueChange={setQuery}
        className="border-b px-2 py-1 pr-10"
      />

      <CommandList className="max-h-max overflow-y-auto">
        {filteredLedgers.length === 0 ? (
          <CommandEmpty>
            {intl.formatMessage({
              id: 'common.noOptions',
              defaultMessage: 'No options found.'
            })}
          </CommandEmpty>
        ) : (
          <CommandGroup>
            {displayedLedgers.map((ledger) => (
              <CommandItem
                key={ledger.id}
                onSelect={() => onSelect(ledger.id)}
                className="truncate"
              >
                {ledger.name}
              </CommandItem>
            ))}

            {!query && hasMore && (
              <div className="border-t border-gray-100 p-1">
                <Button onClick={loadMore} variant="outline" className="w-full">
                  {intl.formatMessage({
                    id: 'common.loadMore',
                    defaultMessage: 'Load more...'
                  })}
                </Button>
              </div>
            )}
          </CommandGroup>
        )}
      </CommandList>
    </Command>
  )
}

export const LedgerSelector = () => {
  const intl = useIntl()
  const [openCommand, setOpenCommand] = React.useState(false)
  const { currentOrganization, currentLedger, setLedger } = useOrganization()

  const { data: ledgers } = useListLedgers({
    organizationId: currentOrganization?.id!,
    limit: 100
  })

  React.useEffect(() => {
    if (ledgers?.items?.length) {
      if (!currentLedger?.id) {
        setLedger(ledgers.items[0])
        return
      }

      const ledgerExists = ledgers.items.some(
        (ledger: LedgerResponseDto) => ledger.id === currentLedger.id
      )

      if (!ledgerExists) {
        setLedger(ledgers.items[0])
      }
    }
  }, [ledgers, currentLedger?.id, setLedger])

  const hasLedgers = !!ledgers?.items?.length
  const totalLedgers = ledgers?.items?.length ?? 0
  const isLargeList = totalLedgers >= 10
  const isSingle = totalLedgers === 1

  if (isSingle) {
    return (
      <Button
        disabled
        className="flex cursor-default items-center gap-4 disabled:opacity-100"
        variant="outline"
      >
        <Database size={20} className="text-zinc-400" />
        <span className="pt-[2px] text-xs font-normal uppercase text-zinc-400">
          {intl.formatMessage({
            id: 'ledger.selector.currentLedger.label',
            defaultMessage: 'Current Ledger'
          })}
        </span>
        <span className="text-sm font-semibold text-zinc-800">
          {ledgers?.items[0].name}
        </span>
      </Button>
    )
  }

  const handleSelectChange = (id: string) => {
    setLedger(ledgers?.items.find((ledger) => ledger.id === id)!)
  }

  const handleCommandChange = (id: string) => {
    setLedger(ledgers?.items.find((ledger) => ledger.id === id)!)
    setOpenCommand(false)
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <div>
            <Select
              value={currentLedger?.id ?? undefined}
              onValueChange={handleSelectChange}
              onOpenChange={(open) => !open && setOpenCommand(false)}
              disabled={!hasLedgers}
            >
              <SelectTrigger className="w-fit text-sm font-semibold text-zinc-800">
                <div className="flex items-center gap-4">
                  <Database size={20} className="text-zinc-400" />
                  <span className="pt-[2px] text-xs font-normal uppercase text-zinc-400">
                    {intl.formatMessage({
                      id: 'ledger.selector.currentLedger.label',
                      defaultMessage: 'Current Ledger'
                    })}
                  </span>
                  <SelectValue placeholder="Select a ledger" />
                </div>
              </SelectTrigger>

              <SelectContent className="w-[var(--radix-select-trigger-width)]">
                {isLargeList ? (
                  <SelectGroup className="px-3 pb-3">
                    <SelectLabel className="text-xs font-medium uppercase text-zinc-400">
                      {intl.formatMessage({
                        id: 'ledgers.title',
                        defaultMessage: 'Ledgers'
                      })}
                    </SelectLabel>
                    <SelectItem
                      disabled
                      value={currentLedger?.id}
                      className="font-medium text-zinc-800 data-[disabled]:opacity-100"
                    >
                      {ledgers?.items?.find(
                        (ledger: any) => ledger.id === currentLedger?.id
                      )?.name ||
                        intl.formatMessage({
                          id: 'ledger.selector.placeholder',
                          defaultMessage: 'Select Ledger'
                        })}
                    </SelectItem>

                    <div className="mt-2">
                      <Button
                        type="button"
                        variant="outline"
                        className="flex w-full justify-start rounded-lg border p-2"
                        onClick={() => setOpenCommand((prev) => !prev)}
                        icon={<ChevronsUpDown className="text-zinc-400" />}
                        iconPlacement="far-end"
                      >
                        {intl.formatMessage({
                          id: 'ledger.selector.selectAnother.label',
                          defaultMessage: 'Select another...'
                        })}
                      </Button>

                      {openCommand && (
                        <div
                          className="my-3 w-fit rounded-lg border"
                          onClick={(e) => e.stopPropagation()}
                          onKeyDown={(e) => e.stopPropagation()}
                        >
                          <LedgerCommand
                            ledgers={ledgers!.items}
                            onSelect={handleCommandChange}
                          />
                        </div>
                      )}
                    </div>
                  </SelectGroup>
                ) : (
                  <SelectGroup className="px-3 pb-3">
                    <SelectLabel className="text-xs font-medium uppercase text-zinc-400">
                      {intl.formatMessage({
                        id: 'ledgers.title',
                        defaultMessage: 'Ledgers'
                      })}
                    </SelectLabel>
                    {ledgers?.items?.map((ledger: any) => (
                      <SelectItem key={ledger.id} value={ledger.id}>
                        {ledger.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                )}
              </SelectContent>
            </Select>
          </div>
        </TooltipTrigger>

        {!hasLedgers && (
          <TooltipContent side="bottom">
            {intl.formatMessage({
              id: 'ledger.selector.noledgers',
              defaultMessage: 'No ledgers available. Please create one first.'
            })}
          </TooltipContent>
        )}
      </Tooltip>
    </TooltipProvider>
  )
}
