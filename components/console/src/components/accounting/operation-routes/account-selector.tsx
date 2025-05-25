'use client'

import { useState, useMemo } from 'react'
import { Check, ChevronDown, Building, Search, Filter } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger
} from '@/components/ui/popover'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  mockAccountTypes,
  type AccountType
} from '@/components/accounting/mock/transaction-route-mock-data'

interface AccountSelectorProps {
  value?: string
  onValueChange: (value: string) => void
  placeholder?: string
  disabled?: boolean
  showHierarchy?: boolean
  filterBy?: {
    nature?: 'debit' | 'credit'
    category?: 'asset' | 'liability' | 'equity' | 'revenue' | 'expense'
    domain?: 'customer' | 'provider' | 'system'
  }
}

const categoryColors = {
  asset: 'bg-blue-100 text-blue-800 border-blue-200',
  liability: 'bg-red-100 text-red-800 border-red-200',
  equity: 'bg-purple-100 text-purple-800 border-purple-200',
  revenue: 'bg-green-100 text-green-800 border-green-200',
  expense: 'bg-orange-100 text-orange-800 border-orange-200'
}

const natureColors = {
  debit: 'bg-red-50 text-red-700 border-red-200',
  credit: 'bg-green-50 text-green-700 border-green-200'
}

const domainColors = {
  customer: 'bg-blue-50 text-blue-700 border-blue-200',
  provider: 'bg-purple-50 text-purple-700 border-purple-200',
  system: 'bg-gray-50 text-gray-700 border-gray-200'
}

export function AccountSelector({
  value,
  onValueChange,
  placeholder = 'Select account type...',
  disabled = false,
  showHierarchy = false,
  filterBy
}: AccountSelectorProps) {
  const [open, setOpen] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [categoryFilter, setCategoryFilter] = useState<string>('all')
  const [domainFilter, setDomainFilter] = useState<string>('all')

  const filteredAccountTypes = useMemo(() => {
    let filtered = mockAccountTypes

    // Apply prop filters
    if (filterBy?.nature) {
      filtered = filtered.filter((at) => at.nature === filterBy.nature)
    }
    if (filterBy?.category) {
      filtered = filtered.filter((at) => at.category === filterBy.category)
    }
    if (filterBy?.domain) {
      filtered = filtered.filter((at) => at.domain === filterBy.domain)
    }

    // Apply UI filters
    if (categoryFilter !== 'all') {
      filtered = filtered.filter((at) => at.category === categoryFilter)
    }
    if (domainFilter !== 'all') {
      filtered = filtered.filter((at) => at.domain === domainFilter)
    }

    // Apply search
    if (searchTerm) {
      filtered = filtered.filter(
        (at) =>
          at.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
          at.code.toLowerCase().includes(searchTerm.toLowerCase()) ||
          at.description.toLowerCase().includes(searchTerm.toLowerCase())
      )
    }

    return filtered
  }, [filterBy, categoryFilter, domainFilter, searchTerm])

  const groupedAccountTypes = useMemo(() => {
    const groups = filteredAccountTypes.reduce(
      (acc, accountType) => {
        const key = showHierarchy ? accountType.category : 'all'
        if (!acc[key]) {
          acc[key] = []
        }
        acc[key].push(accountType)
        return acc
      },
      {} as Record<string, AccountType[]>
    )

    // Sort within each group
    Object.keys(groups).forEach((key) => {
      groups[key].sort((a, b) => a.name.localeCompare(b.name))
    })

    return groups
  }, [filteredAccountTypes, showHierarchy])

  const selectedAccountType = mockAccountTypes.find((at) => at.id === value)

  const handleSelect = (accountTypeId: string) => {
    onValueChange(accountTypeId)
    setOpen(false)
  }

  const renderAccountTypeCard = (accountType: AccountType) => (
    <Card
      key={accountType.id}
      className="cursor-pointer transition-shadow hover:shadow-sm"
    >
      <CardHeader className="pb-2">
        <div className="flex items-start justify-between">
          <div className="space-y-1">
            <CardTitle className="text-sm">{accountType.name}</CardTitle>
            <div className="flex items-center space-x-2">
              <Badge variant="outline" className="text-xs">
                {accountType.code}
              </Badge>
              <Badge
                className={`text-xs ${categoryColors[accountType.category]}`}
              >
                {accountType.category}
              </Badge>
            </div>
          </div>
          {value === accountType.id && (
            <Check className="h-4 w-4 text-primary" />
          )}
        </div>
      </CardHeader>
      <CardContent className="pt-0">
        <div className="space-y-2">
          <CardDescription className="text-xs">
            {accountType.description}
          </CardDescription>
          <div className="flex items-center justify-between">
            <Badge className={`text-xs ${natureColors[accountType.nature]}`}>
              {accountType.nature}
            </Badge>
            <Badge className={`text-xs ${domainColors[accountType.domain]}`}>
              {accountType.domain}
            </Badge>
          </div>
        </div>
      </CardContent>
    </Card>
  )

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className="w-full justify-between"
        >
          <div className="flex items-center space-x-2">
            <Building className="h-4 w-4 shrink-0 text-muted-foreground" />
            {selectedAccountType ? (
              <div className="flex items-center space-x-2">
                <span>{selectedAccountType.name}</span>
                <Badge variant="outline" className="text-xs">
                  {selectedAccountType.code}
                </Badge>
              </div>
            ) : (
              <span className="text-muted-foreground">{placeholder}</span>
            )}
          </div>
          <ChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[600px] p-0" align="start">
        <div className="space-y-4 p-4">
          <div className="flex items-center space-x-2">
            <div className="flex-1">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
                <Input
                  placeholder="Search account types..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="pl-10"
                />
              </div>
            </div>
            <Select value={categoryFilter} onValueChange={setCategoryFilter}>
              <SelectTrigger className="w-[120px]">
                <Filter className="mr-2 h-4 w-4" />
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                <SelectItem value="asset">Asset</SelectItem>
                <SelectItem value="liability">Liability</SelectItem>
                <SelectItem value="equity">Equity</SelectItem>
                <SelectItem value="revenue">Revenue</SelectItem>
                <SelectItem value="expense">Expense</SelectItem>
              </SelectContent>
            </Select>
            <Select value={domainFilter} onValueChange={setDomainFilter}>
              <SelectTrigger className="w-[120px]">
                <Filter className="mr-2 h-4 w-4" />
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                <SelectItem value="customer">Customer</SelectItem>
                <SelectItem value="provider">Provider</SelectItem>
                <SelectItem value="system">System</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <Tabs defaultValue="list" className="w-full">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="list">List View</TabsTrigger>
              <TabsTrigger value="cards">Card View</TabsTrigger>
            </TabsList>

            <TabsContent value="list" className="mt-4">
              <Command className="max-h-[300px]">
                <CommandList>
                  <CommandEmpty>No account types found.</CommandEmpty>
                  {showHierarchy ? (
                    Object.entries(groupedAccountTypes).map(
                      ([category, accountTypes]) => (
                        <CommandGroup
                          key={category}
                          heading={
                            category.charAt(0).toUpperCase() + category.slice(1)
                          }
                        >
                          {accountTypes.map((accountType) => (
                            <CommandItem
                              key={accountType.id}
                              value={accountType.id}
                              onSelect={() => handleSelect(accountType.id)}
                              className="flex items-center justify-between p-3"
                            >
                              <div className="space-y-1">
                                <div className="flex items-center space-x-2">
                                  <span className="font-medium">
                                    {accountType.name}
                                  </span>
                                  <Badge variant="outline" className="text-xs">
                                    {accountType.code}
                                  </Badge>
                                </div>
                                <div className="text-xs text-muted-foreground">
                                  {accountType.description}
                                </div>
                                <div className="flex items-center space-x-2">
                                  <Badge
                                    className={`text-xs ${natureColors[accountType.nature]}`}
                                  >
                                    {accountType.nature}
                                  </Badge>
                                  <Badge
                                    className={`text-xs ${domainColors[accountType.domain]}`}
                                  >
                                    {accountType.domain}
                                  </Badge>
                                </div>
                              </div>
                              {value === accountType.id && (
                                <Check className="h-4 w-4 text-primary" />
                              )}
                            </CommandItem>
                          ))}
                        </CommandGroup>
                      )
                    )
                  ) : (
                    <CommandGroup>
                      {filteredAccountTypes.map((accountType) => (
                        <CommandItem
                          key={accountType.id}
                          value={accountType.id}
                          onSelect={() => handleSelect(accountType.id)}
                          className="flex items-center justify-between p-3"
                        >
                          <div className="space-y-1">
                            <div className="flex items-center space-x-2">
                              <span className="font-medium">
                                {accountType.name}
                              </span>
                              <Badge variant="outline" className="text-xs">
                                {accountType.code}
                              </Badge>
                            </div>
                            <div className="text-xs text-muted-foreground">
                              {accountType.description}
                            </div>
                            <div className="flex items-center space-x-2">
                              <Badge
                                className={`text-xs ${categoryColors[accountType.category]}`}
                              >
                                {accountType.category}
                              </Badge>
                              <Badge
                                className={`text-xs ${natureColors[accountType.nature]}`}
                              >
                                {accountType.nature}
                              </Badge>
                              <Badge
                                className={`text-xs ${domainColors[accountType.domain]}`}
                              >
                                {accountType.domain}
                              </Badge>
                            </div>
                          </div>
                          {value === accountType.id && (
                            <Check className="h-4 w-4 text-primary" />
                          )}
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  )}
                </CommandList>
              </Command>
            </TabsContent>

            <TabsContent value="cards" className="mt-4">
              <div className="max-h-[300px] overflow-y-auto">
                <div className="grid grid-cols-1 gap-2">
                  {filteredAccountTypes.map((accountType) => (
                    <div
                      key={accountType.id}
                      onClick={() => handleSelect(accountType.id)}
                      className={`${value === accountType.id ? 'ring-2 ring-primary' : ''}`}
                    >
                      {renderAccountTypeCard(accountType)}
                    </div>
                  ))}
                </div>
              </div>
            </TabsContent>
          </Tabs>
        </div>
      </PopoverContent>
    </Popover>
  )
}

// Simplified version for inline use
export function SimpleAccountSelector({
  value,
  onValueChange,
  placeholder,
  disabled,
  filterBy
}: AccountSelectorProps) {
  const filteredAccountTypes = useMemo(() => {
    let filtered = mockAccountTypes

    if (filterBy?.nature) {
      filtered = filtered.filter((at) => at.nature === filterBy.nature)
    }
    if (filterBy?.category) {
      filtered = filtered.filter((at) => at.category === filterBy.category)
    }
    if (filterBy?.domain) {
      filtered = filtered.filter((at) => at.domain === filterBy.domain)
    }

    return filtered
  }, [filterBy])

  return (
    <Select value={value} onValueChange={onValueChange} disabled={disabled}>
      <SelectTrigger>
        <Building className="mr-2 h-4 w-4 shrink-0 text-muted-foreground" />
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        {filteredAccountTypes.map((accountType) => (
          <SelectItem key={accountType.id} value={accountType.id}>
            <div className="flex items-center space-x-2">
              <span>{accountType.name}</span>
              <Badge variant="outline" className="text-xs">
                {accountType.code}
              </Badge>
            </div>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
