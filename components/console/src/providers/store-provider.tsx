'use client'

import { ReactNode } from 'react'

// This is a simple provider wrapper for Zustand stores
// Zustand stores don't need providers, but this helps with:
// 1. Initializing stores with server data if needed
// 2. Providing a clear structure for all providers
// 3. Future hydration support for SSR

export function StoreProvider({ children }: { children: ReactNode }) {
  // Could initialize stores with server data here if needed
  // Example: useUIStore.setState({ theme: serverTheme })
  
  return <>{children}</>
}