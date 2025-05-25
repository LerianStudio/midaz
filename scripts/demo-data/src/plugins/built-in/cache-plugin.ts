/**
 * Cache Plugin
 * Caches frequently accessed data to improve performance
 */

import { BasePlugin, PluginContext, EntityContext } from '../internal/plugin-interface';

interface CacheEntry<T> {
  data: T;
  timestamp: number;
  hits: number;
}

interface CacheStats {
  hits: number;
  misses: number;
  evictions: number;
  currentSize: number;
}

export class CachePlugin extends BasePlugin {
  name = 'CachePlugin';
  version = '1.0.0';
  priority = 30;

  private cache = new Map<string, CacheEntry<any>>();
  private maxCacheSize = 1000;
  private ttl = 5 * 60 * 1000; // 5 minutes
  private stats: CacheStats = {
    hits: 0,
    misses: 0,
    evictions: 0,
    currentSize: 0,
  };

  async onInit(context: PluginContext): Promise<void> {
    // Configure cache from context if available
    if (context.config?.cache) {
      this.maxCacheSize = context.config.cache.maxSize || this.maxCacheSize;
      this.ttl = context.config.cache.ttl || this.ttl;
    }

    context.logger.debug(`CachePlugin initialized (maxSize: ${this.maxCacheSize}, ttl: ${this.ttl}ms)`);
  }

  /**
   * Get data from cache
   */
  get<T>(key: string): T | undefined {
    const entry = this.cache.get(key);

    if (!entry) {
      this.stats.misses++;
      return undefined;
    }

    // Check if entry has expired
    if (Date.now() - entry.timestamp > this.ttl) {
      this.cache.delete(key);
      this.stats.currentSize--;
      this.stats.misses++;
      return undefined;
    }

    entry.hits++;
    this.stats.hits++;
    return entry.data as T;
  }

  /**
   * Set data in cache
   */
  set<T>(key: string, data: T): void {
    // Check if we need to evict entries
    if (this.cache.size >= this.maxCacheSize) {
      this.evictLRU();
    }

    this.cache.set(key, {
      data,
      timestamp: Date.now(),
      hits: 0,
    });

    this.stats.currentSize = this.cache.size;
  }

  /**
   * Check if key exists in cache
   */
  has(key: string): boolean {
    const entry = this.cache.get(key);
    
    if (!entry) return false;
    
    // Check expiration
    if (Date.now() - entry.timestamp > this.ttl) {
      this.cache.delete(key);
      this.stats.currentSize--;
      return false;
    }

    return true;
  }

  /**
   * Clear the cache
   */
  clear(): void {
    this.cache.clear();
    this.stats.currentSize = 0;
  }

  /**
   * Evict least recently used entries
   */
  private evictLRU(): void {
    const entriesToEvict = Math.floor(this.maxCacheSize * 0.1); // Evict 10%
    
    // Sort entries by hits (ascending) and timestamp (ascending)
    const sorted = Array.from(this.cache.entries())
      .sort((a, b) => {
        // First sort by hits
        if (a[1].hits !== b[1].hits) {
          return a[1].hits - b[1].hits;
        }
        // Then by age
        return a[1].timestamp - b[1].timestamp;
      });

    // Evict the least used entries
    for (let i = 0; i < entriesToEvict && i < sorted.length; i++) {
      this.cache.delete(sorted[i][0]);
      this.stats.evictions++;
    }
  }

  /**
   * Cache entity lookups
   */
  async afterEntityGeneration(context: EntityContext): Promise<void> {
    // Cache generated entities for quick lookup
    const cacheKey = this.buildEntityCacheKey(context.type, context.entity);
    
    if (cacheKey) {
      this.set(cacheKey, context.entity);
    }

    // Cache by parent relationships
    if (context.parentId) {
      const parentKey = `${context.type}:parent:${context.parentId}`;
      const existing = this.get<any[]>(parentKey) || [];
      existing.push(context.entity);
      this.set(parentKey, existing);
    }
  }

  /**
   * Build cache key for an entity
   */
  private buildEntityCacheKey(type: string, entity: any): string | null {
    // Use entity ID if available
    if (entity.id) {
      return `${type}:${entity.id}`;
    }

    // Use unique identifiers based on entity type
    switch (type) {
      case 'organization':
        return entity.legalDocument ? `${type}:doc:${entity.legalDocument}` : null;
      case 'asset':
        return entity.code ? `${type}:code:${entity.code}` : null;
      case 'account':
        return entity.alias ? `${type}:alias:${entity.alias}` : null;
      default:
        return null;
    }
  }

  /**
   * Get cache statistics
   */
  getStats(): CacheStats {
    return { ...this.stats };
  }

  /**
   * Get cache hit rate
   */
  getHitRate(): number {
    const total = this.stats.hits + this.stats.misses;
    return total > 0 ? (this.stats.hits / total) * 100 : 0;
  }

  /**
   * Cleanup expired entries
   */
  cleanup(): void {
    const now = Date.now();
    let removed = 0;

    for (const [key, entry] of this.cache.entries()) {
      if (now - entry.timestamp > this.ttl) {
        this.cache.delete(key);
        removed++;
      }
    }

    this.stats.currentSize = this.cache.size;
    
    if (removed > 0) {
      const logger = (globalThis as any).logger;
      if (logger) {
        logger.debug(`CachePlugin: Cleaned up ${removed} expired entries`);
      }
    }
  }

  async afterGeneration(): Promise<void> {
    const logger = (globalThis as any).logger;
    
    if (logger) {
      logger.debug(
        `Cache Statistics: ${this.stats.hits} hits, ${this.stats.misses} misses, ` +
        `${this.getHitRate().toFixed(2)}% hit rate, ${this.stats.evictions} evictions`
      );
    }
  }

  async onDestroy(): Promise<void> {
    this.clear();
  }

  /**
   * Utility methods for other components to use the cache
   */
  
  /**
   * Cache a function result
   */
  async cacheFunction<T>(
    key: string,
    fn: () => Promise<T>,
    ttl?: number
  ): Promise<T> {
    const cached = this.get<T>(key);
    
    if (cached !== undefined) {
      return cached;
    }

    const result = await fn();
    this.set(key, result);
    
    return result;
  }

  /**
   * Get entities by parent
   */
  getEntitiesByParent<T>(entityType: string, parentId: string): T[] | undefined {
    return this.get<T[]>(`${entityType}:parent:${parentId}`);
  }

  /**
   * Get entity by unique identifier
   */
  getEntityById<T>(entityType: string, id: string): T | undefined {
    return this.get<T>(`${entityType}:${id}`);
  }
}