import { FeePackageDetails } from '@/core/domain/fee/fee-types'

interface CacheEntry<T> {
  data: T
  timestamp: number
}

/**
 * Simple in-memory cache for fee packages
 * Reduces API calls for frequently accessed packages
 */
export class FeePackageCache {
  private cache: Map<string, CacheEntry<FeePackageDetails>> = new Map()
  private accessOrder: string[] = []

  constructor(
    private options: {
      ttl: number // Time to live in milliseconds
      maxEntries: number
    }
  ) {}

  get(key: string): FeePackageDetails | null {
    const entry = this.cache.get(key)

    if (!entry) {
      return null
    }

    // Check if entry has expired
    if (Date.now() - entry.timestamp > this.options.ttl) {
      this.cache.delete(key)
      this.removeFromAccessOrder(key)
      return null
    }

    // Update access order for LRU
    this.updateAccessOrder(key)

    return entry.data
  }

  set(key: string, data: FeePackageDetails): void {
    // Check if we need to evict old entries
    if (this.cache.size >= this.options.maxEntries && !this.cache.has(key)) {
      this.evictLeastRecentlyUsed()
    }

    this.cache.set(key, {
      data,
      timestamp: Date.now()
    })

    this.updateAccessOrder(key)
  }

  clear(): void {
    this.cache.clear()
    this.accessOrder = []
  }

  size(): number {
    return this.cache.size
  }

  private updateAccessOrder(key: string): void {
    this.removeFromAccessOrder(key)
    this.accessOrder.push(key)
  }

  private removeFromAccessOrder(key: string): void {
    const index = this.accessOrder.indexOf(key)
    if (index > -1) {
      this.accessOrder.splice(index, 1)
    }
  }

  private evictLeastRecentlyUsed(): void {
    if (this.accessOrder.length > 0) {
      const keyToEvict = this.accessOrder.shift()!
      this.cache.delete(keyToEvict)
    }
  }

  // For testing purposes
  getStats(): {
    size: number
    keys: string[]
    ttl: number
    maxEntries: number
  } {
    return {
      size: this.cache.size,
      keys: Array.from(this.cache.keys()),
      ttl: this.options.ttl,
      maxEntries: this.options.maxEntries
    }
  }
}
