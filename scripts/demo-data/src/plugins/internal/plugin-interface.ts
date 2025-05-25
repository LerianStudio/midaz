/**
 * Internal plugin system interface
 * Allows extending functionality without modifying core code
 */

import { GenerationMetrics, GeneratorState } from '../../types';
import { Logger } from '../../services/logger';
import { StateManager } from '../../utils/state';

export interface PluginContext {
  logger: Logger;
  stateManager: StateManager;
  config: any;
}

export interface ErrorContext {
  entityType: string;
  parentId?: string;
  operation: string;
  attempt: number;
  error: Error;
}

export interface EntityContext<T = any> {
  type: string;
  entity: T;
  parentId?: string;
  organizationId?: string;
}

/**
 * Internal plugin interface
 */
export interface InternalPlugin {
  name: string;
  version: string;
  enabled: boolean;
  priority?: number; // Lower numbers run first

  // Lifecycle hooks
  onInit?(context: PluginContext): Promise<void>;
  onDestroy?(): Promise<void>;

  // Generation lifecycle
  beforeGeneration?(config: any): Promise<void>;
  afterGeneration?(metrics: GenerationMetrics): Promise<void>;

  // Entity lifecycle
  beforeEntityGeneration?(type: string, config: any): Promise<void>;
  afterEntityGeneration?(context: EntityContext): Promise<void>;
  onEntityGenerationError?(error: ErrorContext): Promise<void>;

  // State management
  onStateChange?(state: GeneratorState, changeType: string): void;
  onCheckpoint?(checkpointId: string): Promise<void>;
  onRestore?(checkpointId: string): Promise<void>;

  // Performance monitoring
  onMetricsUpdate?(metrics: Partial<GenerationMetrics>): void;
  onMemoryWarning?(stats: any): void;

  // Custom events
  onEvent?(eventName: string, data: any): Promise<void>;
}

/**
 * Base plugin class with default implementations
 */
export abstract class BasePlugin implements InternalPlugin {
  abstract name: string;
  abstract version: string;
  enabled: boolean = true;
  priority: number = 100;

  async onInit(context: PluginContext): Promise<void> {
    // Default: no initialization needed
  }

  async onDestroy(): Promise<void> {
    // Default: no cleanup needed
  }
}

/**
 * Plugin metadata
 */
export interface PluginMetadata {
  name: string;
  version: string;
  description: string;
  author?: string;
  dependencies?: string[];
  tags?: string[];
}