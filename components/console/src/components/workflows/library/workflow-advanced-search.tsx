'use client'

import { useState, useEffect, useRef, useMemo } from 'react'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger
} from '@/components/ui/popover'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'
import {
  Search,
  X,
  Star,
  History,
  Filter,
  Save,
  Trash2,
  Settings2,
  Info,
  Hash,
  User,
  Calendar,
  Activity,
  Tag,
  Folder,
  CheckCircle,
  Clock,
  FileText,
  XCircle
} from 'lucide-react'
import { WorkflowStatus } from '@/core/domain/entities/workflow'
import { useToast } from '@/hooks/use-toast'
import { useDebounce } from '@/hooks/use-debounce'
// @ts-ignore - fuse.js types not available
import Fuse from 'fuse.js'

interface AdvancedSearchProps {
  onSearch: (query: string, filters: SearchFilters) => void
  workflows: Array<{
    id: string
    name: string
    description?: string
    status: WorkflowStatus
    metadata: {
      tags: string[]
      category?: string
      author?: string
    }
    createdBy: string
    executionCount: number
  }>
  className?: string
}

export interface SearchFilters {
  status?: WorkflowStatus[]
  categories?: string[]
  tags?: string[]
  authors?: string[]
  executionRange?: { min?: number; max?: number }
  dateRange?: { from?: Date; to?: Date }
}

interface SearchPreset {
  id: string
  name: string
  query: string
  filters: SearchFilters
  icon?: React.ReactNode
}

interface SearchHistory {
  id: string
  query: string
  filters: SearchFilters
  timestamp: Date
}

interface FuseResult {
  item: any
  score?: number
  matches?: any[]
}

const DEFAULT_PRESETS: SearchPreset[] = [
  {
    id: 'active-workflows',
    name: 'Active Workflows',
    query: '',
    filters: { status: ['ACTIVE' as WorkflowStatus] },
    icon: <CheckCircle className="h-4 w-4" />
  },
  {
    id: 'high-usage',
    name: 'High Usage',
    query: '',
    filters: { executionRange: { min: 100 } },
    icon: <Activity className="h-4 w-4" />
  },
  {
    id: 'payment-workflows',
    name: 'Payment Workflows',
    query: 'payment',
    filters: { categories: ['payments'] },
    icon: <Tag className="h-4 w-4" />
  },
  {
    id: 'draft-workflows',
    name: 'Drafts',
    query: '',
    filters: { status: ['DRAFT' as WorkflowStatus] },
    icon: <FileText className="h-4 w-4" />
  }
]

const SEARCH_SYNTAX_HELP = [
  { syntax: 'tag:payment', description: 'Search by tag' },
  { syntax: 'status:active', description: 'Filter by status' },
  { syntax: 'author:john', description: 'Filter by author' },
  { syntax: 'category:onboarding', description: 'Filter by category' },
  { syntax: 'runs:>100', description: 'Filter by execution count' },
  { syntax: '"exact phrase"', description: 'Search exact phrase' }
]

export function WorkflowAdvancedSearch({
  onSearch,
  workflows,
  className
}: AdvancedSearchProps) {
  const { toast } = useToast()
  const [query, setQuery] = useState('')
  const [filters, setFilters] = useState<SearchFilters>({})
  const [showSuggestions, setShowSuggestions] = useState(false)
  const [showHelp, setShowHelp] = useState(false)
  const [showSaveDialog, setShowSaveDialog] = useState(false)
  const [presetName, setPresetName] = useState('')
  const [savedPresets, setSavedPresets] = useState<SearchPreset[]>([])
  const [searchHistory, setSearchHistory] = useState<SearchHistory[]>([])
  const [highlightedIndex, setHighlightedIndex] = useState(-1)
  const inputRef = useRef<HTMLInputElement>(null)
  const [debouncedQuery, setDebouncedQuery] = useState(query)

  useDebounce(
    () => {
      setDebouncedQuery(query)
    },
    300,
    [query]
  )

  // Load saved presets and history from localStorage
  useEffect(() => {
    const saved = localStorage.getItem('workflow-search-presets')
    if (saved) {
      setSavedPresets(JSON.parse(saved))
    }

    const history = localStorage.getItem('workflow-search-history')
    if (history) {
      setSearchHistory(JSON.parse(history))
    }
  }, [])

  // Parse advanced query syntax
  const parseQuery = (
    searchQuery: string
  ): { query: string; filters: SearchFilters } => {
    const parsedFilters: SearchFilters = {}
    let cleanQuery = searchQuery

    // Extract tag filters
    const tagMatches = searchQuery.match(/tag:(\S+)/g)
    if (tagMatches) {
      parsedFilters.tags = tagMatches.map((m) => m.replace('tag:', ''))
      cleanQuery = cleanQuery.replace(/tag:\S+/g, '')
    }

    // Extract status filters
    const statusMatch = searchQuery.match(/status:(\S+)/i)
    if (statusMatch) {
      const status = statusMatch[1].toUpperCase() as WorkflowStatus
      parsedFilters.status = [status]
      cleanQuery = cleanQuery.replace(/status:\S+/gi, '')
    }

    // Extract author filters
    const authorMatch = searchQuery.match(/author:(\S+)/i)
    if (authorMatch) {
      parsedFilters.authors = [authorMatch[1]]
      cleanQuery = cleanQuery.replace(/author:\S+/gi, '')
    }

    // Extract category filters
    const categoryMatch = searchQuery.match(/category:(\S+)/i)
    if (categoryMatch) {
      parsedFilters.categories = [categoryMatch[1]]
      cleanQuery = cleanQuery.replace(/category:\S+/gi, '')
    }

    // Extract execution count filters
    const runsMatch = searchQuery.match(/runs:([><=])(\d+)/i)
    if (runsMatch) {
      const operator = runsMatch[1]
      const value = parseInt(runsMatch[2])
      if (operator === '>') {
        parsedFilters.executionRange = { min: value }
      } else if (operator === '<') {
        parsedFilters.executionRange = { max: value }
      }
      cleanQuery = cleanQuery.replace(/runs:[><=]\d+/gi, '')
    }

    return {
      query: cleanQuery.trim(),
      filters: parsedFilters
    }
  }

  // Fuzzy search setup
  const fuse = useMemo(() => {
    return new (Fuse as any)(workflows, {
      keys: [
        { name: 'name', weight: 2 },
        { name: 'description', weight: 1 },
        { name: 'metadata.tags', weight: 1.5 },
        { name: 'metadata.category', weight: 1 },
        { name: 'metadata.author', weight: 1 }
      ],
      threshold: 0.3,
      includeScore: true,
      includeMatches: true
    })
  }, [workflows])

  // Generate search suggestions
  const suggestions = useMemo(() => {
    if (!debouncedQuery || debouncedQuery.length < 2) return []

    const parsed = parseQuery(debouncedQuery)
    const results = (fuse.search(parsed.query) as FuseResult[]).slice(0, 5)

    return results.map((result: FuseResult) => ({
      workflow: result.item,
      score: result.score || 0,
      matches: result.matches || []
    }))
  }, [debouncedQuery, fuse])

  // Get unique values for filters
  const uniqueValues = useMemo(() => {
    const tags = new Set<string>()
    const categories = new Set<string>()
    const authors = new Set<string>()

    workflows.forEach((w) => {
      w.metadata.tags.forEach((t) => tags.add(t))
      if (w.metadata.category) categories.add(w.metadata.category)
      const author = w.metadata.author || w.createdBy
      if (author) authors.add(author)
    })

    return {
      tags: Array.from(tags),
      categories: Array.from(categories),
      authors: Array.from(authors)
    }
  }, [workflows])

  // Handle search execution
  const executeSearch = () => {
    const parsed = parseQuery(query)
    const combinedFilters = { ...filters, ...parsed.filters }

    onSearch(parsed.query, combinedFilters)

    // Add to search history
    const historyItem: SearchHistory = {
      id: Date.now().toString(),
      query: query,
      filters: combinedFilters,
      timestamp: new Date()
    }

    const newHistory = [historyItem, ...searchHistory.slice(0, 9)]
    setSearchHistory(newHistory)
    localStorage.setItem('workflow-search-history', JSON.stringify(newHistory))

    setShowSuggestions(false)
  }

  // Handle preset selection
  const applyPreset = (preset: SearchPreset) => {
    setQuery(preset.query)
    setFilters(preset.filters)
    onSearch(preset.query, preset.filters)
  }

  // Save current search as preset
  const savePreset = () => {
    if (!presetName.trim()) {
      toast({
        title: 'Error',
        description: 'Please enter a name for the preset',
        variant: 'destructive'
      })
      return
    }

    const preset: SearchPreset = {
      id: Date.now().toString(),
      name: presetName,
      query: query,
      filters: filters,
      icon: <Star className="h-4 w-4" />
    }

    const newPresets = [...savedPresets, preset]
    setSavedPresets(newPresets)
    localStorage.setItem('workflow-search-presets', JSON.stringify(newPresets))

    toast({
      title: 'Success',
      description: 'Search preset saved successfully'
    })

    setShowSaveDialog(false)
    setPresetName('')
  }

  // Delete preset
  const deletePreset = (presetId: string) => {
    const newPresets = savedPresets.filter((p) => p.id !== presetId)
    setSavedPresets(newPresets)
    localStorage.setItem('workflow-search-presets', JSON.stringify(newPresets))

    toast({
      title: 'Success',
      description: 'Preset deleted successfully'
    })
  }

  // Clear search history
  const clearHistory = () => {
    setSearchHistory([])
    localStorage.removeItem('workflow-search-history')
    toast({
      title: 'Success',
      description: 'Search history cleared'
    })
  }

  // Keyboard navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!showSuggestions) return

      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setHighlightedIndex((prev) =>
          prev < suggestions.length - 1 ? prev + 1 : prev
        )
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        setHighlightedIndex((prev) => (prev > 0 ? prev - 1 : -1))
      } else if (e.key === 'Enter' && highlightedIndex >= 0) {
        e.preventDefault()
        const suggestion = suggestions[highlightedIndex]
        if (suggestion) {
          setQuery(suggestion.workflow.name)
          executeSearch()
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [showSuggestions, suggestions, highlightedIndex])

  // Highlight matching text
  const highlightMatch = (text: string, matches?: any[]) => {
    if (!matches) return text

    const relevantMatch = matches.find((m) => m.value === text)
    if (!relevantMatch) return text

    const { indices } = relevantMatch
    let lastIndex = 0
    const parts: React.ReactNode[] = []

    indices.forEach(([start, end]: [number, number], i: number) => {
      if (start > lastIndex) {
        parts.push(
          <span key={`normal-${i}`}>{text.slice(lastIndex, start)}</span>
        )
      }
      parts.push(
        <span key={`highlight-${i}`} className="font-semibold text-primary">
          {text.slice(start, end + 1)}
        </span>
      )
      lastIndex = end + 1
    })

    if (lastIndex < text.length) {
      parts.push(<span key="normal-last">{text.slice(lastIndex)}</span>)
    }

    return <>{parts}</>
  }

  return (
    <div className={cn('relative', className)}>
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
          <Input
            ref={inputRef}
            placeholder="Search workflows... (try 'tag:payment' or 'status:active')"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setShowSuggestions(true)
            }}
            onFocus={() => setShowSuggestions(true)}
            onBlur={() => setTimeout(() => setShowSuggestions(false), 200)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && highlightedIndex === -1) {
                executeSearch()
              }
            }}
            className="pl-10 pr-10"
          />
          {query && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setQuery('')
                setFilters({})
                onSearch('', {})
              }}
              className="absolute right-2 top-1/2 h-6 w-6 -translate-y-1/2 transform p-0"
            >
              <X className="h-4 w-4" />
            </Button>
          )}
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="icon">
              <Filter className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent className="w-56">
            <DropdownMenuLabel>Search Presets</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {DEFAULT_PRESETS.map((preset) => (
              <DropdownMenuItem
                key={preset.id}
                onClick={() => applyPreset(preset)}
              >
                {preset.icon}
                <span className="ml-2">{preset.name}</span>
              </DropdownMenuItem>
            ))}
            {savedPresets.length > 0 && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuLabel>Saved Searches</DropdownMenuLabel>
                {savedPresets.map((preset) => (
                  <DropdownMenuItem
                    key={preset.id}
                    onClick={() => applyPreset(preset)}
                    className="group"
                  >
                    <Star className="h-4 w-4" />
                    <span className="ml-2 flex-1">{preset.name}</span>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={(e) => {
                        e.stopPropagation()
                        deletePreset(preset.id)
                      }}
                      className="invisible h-5 w-5 p-0 group-hover:visible"
                    >
                      <Trash2 className="h-3 w-3" />
                    </Button>
                  </DropdownMenuItem>
                ))}
              </>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => setShowSaveDialog(true)}>
              <Save className="mr-2 h-4 w-4" />
              Save Current Search
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <Button variant="outline" size="icon" onClick={() => setShowHelp(true)}>
          <Info className="h-4 w-4" />
        </Button>
      </div>

      {/* Search Suggestions Dropdown */}
      {showSuggestions && (query || searchHistory.length > 0) && (
        <div className="absolute left-0 right-0 top-full z-50 mt-1 rounded-md border bg-popover p-0 shadow-md">
          <ScrollArea className="max-h-[400px]">
            {suggestions.length > 0 && (
              <>
                <div className="p-2">
                  <p className="mb-2 text-xs font-medium text-muted-foreground">
                    Search Results
                  </p>
                  {suggestions.map((suggestion: any, index: number) => (
                    <div
                      key={suggestion.workflow.id}
                      className={cn(
                        'cursor-pointer rounded-sm px-2 py-1.5 text-sm hover:bg-accent',
                        highlightedIndex === index && 'bg-accent'
                      )}
                      onClick={() => {
                        setQuery(suggestion.workflow.name)
                        executeSearch()
                      }}
                    >
                      <div className="font-medium">
                        {highlightMatch(
                          suggestion.workflow.name,
                          suggestion.matches
                        )}
                      </div>
                      {suggestion.workflow.description && (
                        <div className="text-xs text-muted-foreground">
                          {suggestion.workflow.description}
                        </div>
                      )}
                      <div className="mt-1 flex items-center gap-2">
                        <Badge variant="secondary" className="text-xs">
                          {suggestion.workflow.status}
                        </Badge>
                        {suggestion.workflow.metadata.category && (
                          <Badge variant="outline" className="text-xs">
                            {suggestion.workflow.metadata.category}
                          </Badge>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
                {searchHistory.length > 0 && <Separator />}
              </>
            )}

            {searchHistory.length > 0 && !query && (
              <div className="p-2">
                <div className="mb-2 flex items-center justify-between">
                  <p className="text-xs font-medium text-muted-foreground">
                    Recent Searches
                  </p>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={clearHistory}
                    className="h-6 px-2 text-xs"
                  >
                    Clear
                  </Button>
                </div>
                {searchHistory.slice(0, 5).map((item) => (
                  <div
                    key={item.id}
                    className="cursor-pointer rounded-sm px-2 py-1.5 text-sm hover:bg-accent"
                    onClick={() => {
                      setQuery(item.query)
                      setFilters(item.filters)
                      executeSearch()
                    }}
                  >
                    <div className="flex items-center gap-2">
                      <History className="h-3 w-3 text-muted-foreground" />
                      <span>{item.query || 'Advanced filters'}</span>
                    </div>
                  </div>
                ))}
              </div>
            )}

            <Separator />
            <div className="p-2">
              <p className="mb-2 text-xs font-medium text-muted-foreground">
                Quick Filters
              </p>
              <div className="flex flex-wrap gap-1">
                {uniqueValues.tags.slice(0, 5).map((tag) => (
                  <Badge
                    key={tag}
                    variant="outline"
                    className="cursor-pointer text-xs"
                    onClick={() => {
                      setQuery(`tag:${tag}`)
                      executeSearch()
                    }}
                  >
                    <Hash className="mr-1 h-3 w-3" />
                    {tag}
                  </Badge>
                ))}
              </div>
            </div>
          </ScrollArea>
        </div>
      )}

      {/* Save Preset Dialog */}
      <Dialog open={showSaveDialog} onOpenChange={setShowSaveDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Save Search Preset</DialogTitle>
            <DialogDescription>
              Save your current search and filters as a preset for quick access
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <label htmlFor="preset-name" className="text-sm font-medium">
                Preset Name
              </label>
              <Input
                id="preset-name"
                value={presetName}
                onChange={(e) => setPresetName(e.target.value)}
                placeholder="e.g., Active Payment Workflows"
              />
            </div>
            <div className="grid gap-2">
              <p className="text-sm font-medium">Current Search</p>
              <div className="rounded-md border p-3">
                <p className="text-sm">
                  <span className="font-medium">Query:</span> {query || 'None'}
                </p>
                {Object.keys(filters).length > 0 && (
                  <p className="mt-1 text-sm">
                    <span className="font-medium">Filters:</span>{' '}
                    {JSON.stringify(filters)}
                  </p>
                )}
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowSaveDialog(false)}>
              Cancel
            </Button>
            <Button onClick={savePreset}>Save Preset</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Search Help Dialog */}
      <Dialog open={showHelp} onOpenChange={setShowHelp}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Advanced Search Help</DialogTitle>
            <DialogDescription>
              Learn how to use advanced search operators to find workflows
              quickly
            </DialogDescription>
          </DialogHeader>
          <ScrollArea className="max-h-[400px] pr-4">
            <div className="space-y-4">
              <div>
                <h4 className="mb-2 font-medium">Search Operators</h4>
                <div className="space-y-2">
                  {SEARCH_SYNTAX_HELP.map((item, index) => (
                    <div
                      key={index}
                      className="flex items-center justify-between rounded-md border p-2"
                    >
                      <code className="rounded bg-muted px-1.5 py-0.5 text-sm">
                        {item.syntax}
                      </code>
                      <span className="text-sm text-muted-foreground">
                        {item.description}
                      </span>
                    </div>
                  ))}
                </div>
              </div>

              <div>
                <h4 className="mb-2 font-medium">Examples</h4>
                <div className="space-y-2 text-sm">
                  <p>
                    <code className="rounded bg-muted px-1.5 py-0.5">
                      payment status:active
                    </code>{' '}
                    - Find active workflows with &quot;payment&quot; in the name
                  </p>
                  <p>
                    <code className="rounded bg-muted px-1.5 py-0.5">
                      tag:onboarding author:john
                    </code>{' '}
                    - Find workflows tagged &quot;onboarding&quot; by author
                    &quot;john&quot;
                  </p>
                  <p>
                    <code className="rounded bg-muted px-1.5 py-0.5">
                      runs:&gt;100 category:compliance
                    </code>{' '}
                    - Find compliance workflows with over 100 executions
                  </p>
                </div>
              </div>

              <div>
                <h4 className="mb-2 font-medium">Tips</h4>
                <ul className="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
                  <li>Use quotes for exact phrase matching</li>
                  <li>Combine multiple operators for precise filtering</li>
                  <li>Save frequently used searches as presets</li>
                  <li>The search is fuzzy and handles typos automatically</li>
                </ul>
              </div>
            </div>
          </ScrollArea>
          <DialogFooter>
            <Button onClick={() => setShowHelp(false)}>Got it</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
