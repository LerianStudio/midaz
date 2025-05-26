import { create } from 'zustand'
import { devtools, persist, subscribeWithSelector } from 'zustand/middleware'
import { immer } from 'zustand/middleware/immer'

// Global UI State
interface UIState {
  theme: 'light' | 'dark' | 'system'
  sidebarCollapsed: boolean
  commandPaletteOpen: boolean
  globalSearchQuery: string
  recentSearches: string[]
  activeFilters: Record<string, any>
  
  // Actions
  setTheme: (theme: UIState['theme']) => void
  toggleSidebar: () => void
  toggleCommandPalette: () => void
  setGlobalSearchQuery: (query: string) => void
  addRecentSearch: (search: string) => void
  setActiveFilter: (key: string, value: any) => void
  clearFilters: () => void
}

export const useUIStore = create<UIState>()(
  devtools(
    persist(
      immer((set) => ({
        theme: 'system',
        sidebarCollapsed: false,
        commandPaletteOpen: false,
        globalSearchQuery: '',
        recentSearches: [],
        activeFilters: {},
        
        setTheme: (theme) => set((state) => {
          state.theme = theme
        }),
        
        toggleSidebar: () => set((state) => {
          state.sidebarCollapsed = !state.sidebarCollapsed
        }),
        
        toggleCommandPalette: () => set((state) => {
          state.commandPaletteOpen = !state.commandPaletteOpen
        }),
        
        setGlobalSearchQuery: (query) => set((state) => {
          state.globalSearchQuery = query
        }),
        
        addRecentSearch: (search) => set((state) => {
          if (!state.recentSearches.includes(search)) {
            state.recentSearches = [search, ...state.recentSearches].slice(0, 10)
          }
        }),
        
        setActiveFilter: (key, value) => set((state) => {
          if (value === null || value === undefined) {
            delete state.activeFilters[key]
          } else {
            state.activeFilters[key] = value
          }
        }),
        
        clearFilters: () => set((state) => {
          state.activeFilters = {}
        }),
      })),
      {
        name: 'midaz-ui-store',
        partialize: (state) => ({
          theme: state.theme,
          sidebarCollapsed: state.sidebarCollapsed,
          recentSearches: state.recentSearches,
        }),
      }
    )
  )
)

// Real-time Updates State
interface RealtimeState {
  connected: boolean
  connectionStatus: 'connecting' | 'connected' | 'disconnected' | 'error'
  lastHeartbeat: number | null
  subscribedChannels: Set<string>
  pendingUpdates: Map<string, any[]>
  
  // Actions
  setConnected: (connected: boolean) => void
  setConnectionStatus: (status: RealtimeState['connectionStatus']) => void
  updateHeartbeat: () => void
  subscribe: (channel: string) => void
  unsubscribe: (channel: string) => void
  addPendingUpdate: (channel: string, update: any) => void
  clearPendingUpdates: (channel: string) => void
}

export const useRealtimeStore = create<RealtimeState>()(
  subscribeWithSelector(
    immer((set) => ({
      connected: false,
      connectionStatus: 'disconnected',
      lastHeartbeat: null,
      subscribedChannels: new Set(),
      pendingUpdates: new Map(),
      
      setConnected: (connected) => set((state) => {
        state.connected = connected
        state.connectionStatus = connected ? 'connected' : 'disconnected'
      }),
      
      setConnectionStatus: (status) => set((state) => {
        state.connectionStatus = status
      }),
      
      updateHeartbeat: () => set((state) => {
        state.lastHeartbeat = Date.now()
      }),
      
      subscribe: (channel) => set((state) => {
        state.subscribedChannels.add(channel)
      }),
      
      unsubscribe: (channel) => set((state) => {
        state.subscribedChannels.delete(channel)
        state.pendingUpdates.delete(channel)
      }),
      
      addPendingUpdate: (channel, update) => set((state) => {
        const updates = state.pendingUpdates.get(channel) || []
        state.pendingUpdates.set(channel, [...updates, update])
      }),
      
      clearPendingUpdates: (channel) => set((state) => {
        state.pendingUpdates.delete(channel)
      }),
    }))
  )
)

// Performance Monitoring State
interface PerformanceState {
  metrics: {
    renderCount: number
    lastRenderTime: number
    averageRenderTime: number
    slowRenders: number
  }
  
  // Actions
  recordRender: (duration: number) => void
  resetMetrics: () => void
}

export const usePerformanceStore = create<PerformanceState>()(
  immer((set) => ({
    metrics: {
      renderCount: 0,
      lastRenderTime: 0,
      averageRenderTime: 0,
      slowRenders: 0,
    },
    
    recordRender: (duration) => set((state) => {
      const { renderCount, averageRenderTime } = state.metrics
      
      state.metrics.renderCount += 1
      state.metrics.lastRenderTime = duration
      state.metrics.averageRenderTime = 
        (averageRenderTime * renderCount + duration) / (renderCount + 1)
      
      if (duration > 16) { // More than one frame (60fps)
        state.metrics.slowRenders += 1
      }
    }),
    
    resetMetrics: () => set((state) => {
      state.metrics = {
        renderCount: 0,
        lastRenderTime: 0,
        averageRenderTime: 0,
        slowRenders: 0,
      }
    }),
  }))
)