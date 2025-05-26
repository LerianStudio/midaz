'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useUIStore } from '@/store'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from '@/components/ui/command'
import {
  Calculator,
  CreditCard,
  FileText,
  Home,
  LineChart,
  Search,
  Settings,
  UserPlus,
  Wallet,
  ArrowRight,
  Clock,
  Star,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { useDebounce } from '@/hooks/use-debounce'

interface CommandItem {
  id: string
  title: string
  description?: string
  icon: React.ComponentType<{ className?: string }>
  action: () => void
  shortcut?: string
  keywords?: string[]
  category: string
  recent?: boolean
  favorite?: boolean
}

export function CommandPalette() {
  const router = useRouter()
  const { commandPaletteOpen, toggleCommandPalette, recentSearches, addRecentSearch } = useUIStore()
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search, 300)
  
  // Define all available commands
  const commands: CommandItem[] = useMemo(() => [
    // Navigation
    {
      id: 'nav-dashboard',
      title: 'Go to Dashboard',
      description: 'View your main dashboard',
      icon: Home,
      action: () => router.push('/'),
      shortcut: '⌘D',
      keywords: ['home', 'main', 'overview'],
      category: 'Navigation',
    },
    {
      id: 'nav-transactions',
      title: 'View Transactions',
      description: 'Browse all transactions',
      icon: CreditCard,
      action: () => router.push('/transactions'),
      shortcut: '⌘T',
      keywords: ['payments', 'transfers', 'money'],
      category: 'Navigation',
    },
    {
      id: 'nav-accounts',
      title: 'Manage Accounts',
      description: 'View and manage accounts',
      icon: Wallet,
      action: () => router.push('/accounts'),
      keywords: ['wallets', 'balances'],
      category: 'Navigation',
    },
    {
      id: 'nav-analytics',
      title: 'Analytics',
      description: 'View reports and insights',
      icon: LineChart,
      action: () => router.push('/analytics'),
      keywords: ['reports', 'insights', 'statistics'],
      category: 'Navigation',
    },
    
    // Actions
    {
      id: 'action-new-transaction',
      title: 'Create Transaction',
      description: 'Create a new transaction',
      icon: CreditCard,
      action: () => router.push('/transactions/create'),
      shortcut: '⌘N',
      keywords: ['new', 'add', 'payment'],
      category: 'Actions',
    },
    {
      id: 'action-new-account',
      title: 'Create Account',
      description: 'Create a new account',
      icon: UserPlus,
      action: () => router.push('/accounts/create'),
      keywords: ['new', 'add', 'wallet'],
      category: 'Actions',
    },
    {
      id: 'action-calculator',
      title: 'Open Calculator',
      description: 'Quick calculations',
      icon: Calculator,
      action: () => router.push('/tools/calculator'),
      keywords: ['math', 'calculate', 'compute'],
      category: 'Tools',
    },
    {
      id: 'action-export',
      title: 'Export Data',
      description: 'Export transactions or reports',
      icon: FileText,
      action: () => router.push('/export'),
      keywords: ['download', 'csv', 'pdf'],
      category: 'Actions',
    },
    
    // Settings
    {
      id: 'settings-general',
      title: 'General Settings',
      description: 'Configure your preferences',
      icon: Settings,
      action: () => router.push('/settings'),
      shortcut: '⌘,',
      keywords: ['preferences', 'configuration'],
      category: 'Settings',
    },
  ], [router])
  
  // Filter commands based on search
  const filteredCommands = useMemo(() => {
    if (!debouncedSearch) return commands
    
    const searchLower = debouncedSearch.toLowerCase()
    return commands.filter(command => {
      const titleMatch = command.title.toLowerCase().includes(searchLower)
      const descMatch = command.description?.toLowerCase().includes(searchLower)
      const keywordMatch = command.keywords?.some(k => k.includes(searchLower))
      
      return titleMatch || descMatch || keywordMatch
    })
  }, [commands, debouncedSearch])
  
  // Group commands by category
  const groupedCommands = useMemo(() => {
    return filteredCommands.reduce((acc, command) => {
      if (!acc[command.category]) {
        acc[command.category] = []
      }
      acc[command.category].push(command)
      return acc
    }, {} as Record<string, CommandItem[]>)
  }, [filteredCommands])
  
  // Handle command execution
  const executeCommand = useCallback((command: CommandItem) => {
    toggleCommandPalette()
    addRecentSearch(command.title)
    command.action()
  }, [toggleCommandPalette, addRecentSearch])
  
  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Cmd/Ctrl + K to open command palette
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        toggleCommandPalette()
      }
      
      // Direct shortcuts when palette is closed
      if (!commandPaletteOpen) {
        commands.forEach(command => {
          if (command.shortcut) {
            const keys = command.shortcut.toLowerCase().replace('⌘', '').replace('⌥', '')
            if ((e.metaKey || e.ctrlKey) && e.key === keys) {
              e.preventDefault()
              command.action()
            }
          }
        })
      }
    }
    
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [commandPaletteOpen, commands, toggleCommandPalette])
  
  return (
    <CommandDialog open={commandPaletteOpen} onOpenChange={toggleCommandPalette}>
      <CommandInput 
        placeholder="Type a command or search..." 
        value={search}
        onValueChange={setSearch}
      />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
        
        {/* Recent searches */}
        {recentSearches.length > 0 && !search && (
          <>
            <CommandGroup heading="Recent">
              {recentSearches.slice(0, 3).map((item) => (
                <CommandItem
                  key={`recent-${item}`}
                  onSelect={() => setSearch(item)}
                >
                  <Clock className="mr-2 h-4 w-4 text-muted-foreground" />
                  <span>{item}</span>
                </CommandItem>
              ))}
            </CommandGroup>
            <CommandSeparator />
          </>
        )}
        
        {/* Grouped commands */}
        {Object.entries(groupedCommands).map(([category, items]) => (
          <CommandGroup key={category} heading={category}>
            {items.map((command) => {
              const Icon = command.icon
              return (
                <CommandItem
                  key={command.id}
                  onSelect={() => executeCommand(command)}
                >
                  <Icon className="mr-2 h-4 w-4" />
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <span>{command.title}</span>
                      {command.recent && (
                        <Badge variant="secondary" className="h-5 text-xs">
                          Recent
                        </Badge>
                      )}
                      {command.favorite && (
                        <Star className="h-3 w-3 fill-yellow-400 text-yellow-400" />
                      )}
                    </div>
                    {command.description && (
                      <p className="text-xs text-muted-foreground">
                        {command.description}
                      </p>
                    )}
                  </div>
                  {command.shortcut && (
                    <CommandShortcut>{command.shortcut}</CommandShortcut>
                  )}
                </CommandItem>
              )
            })}
          </CommandGroup>
        ))}
        
        {/* Quick actions */}
        <CommandSeparator />
        <CommandGroup heading="Quick Actions">
          <CommandItem onSelect={() => router.push('/search')}>
            <Search className="mr-2 h-4 w-4" />
            <span>Search everything...</span>
            <ArrowRight className="ml-auto h-4 w-4" />
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  )
}