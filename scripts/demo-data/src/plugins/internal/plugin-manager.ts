/**
 * Internal Plugin Manager
 * Manages the lifecycle and execution of internal plugins
 */

import { Logger } from '../../services/logger';
import { StateManager } from '../../utils/state';
import { GenerationMetrics, GeneratorState } from '../../types';
import { 
  InternalPlugin, 
  PluginContext, 
  ErrorContext, 
  EntityContext 
} from './plugin-interface';

// Import built-in plugins
import { MetricsPlugin } from '../built-in/metrics-plugin';
import { ValidationPlugin } from '../built-in/validation-plugin';
import { CachePlugin } from '../built-in/cache-plugin';

export class InternalPluginManager {
  private plugins: InternalPlugin[] = [];
  private context: PluginContext;
  private initialized = false;

  constructor(
    private logger: Logger,
    private stateManager: StateManager,
    private config: any = {}
  ) {
    this.context = {
      logger,
      stateManager,
      config,
    };
  }

  /**
   * Initialize the plugin manager and load built-in plugins
   */
  async initialize(): Promise<void> {
    if (this.initialized) {
      return;
    }

    // Load built-in plugins
    await this.loadBuiltInPlugins();
    this.initialized = true;
  }

  /**
   * Load built-in plugins automatically
   */
  private async loadBuiltInPlugins(): Promise<void> {
    const builtInPlugins: InternalPlugin[] = [
      new MetricsPlugin(),
      new ValidationPlugin(),
      new CachePlugin(),
    ];

    for (const plugin of builtInPlugins) {
      if (plugin.enabled) {
        await this.registerPlugin(plugin);
      }
    }

    this.logger.debug(`Loaded ${this.plugins.length} built-in plugins`);
  }

  /**
   * Register a plugin
   */
  async registerPlugin(plugin: InternalPlugin): Promise<void> {
    try {
      // Initialize the plugin
      if (plugin.onInit) {
        await plugin.onInit(this.context);
      }

      // Add to plugins list (sorted by priority)
      this.plugins.push(plugin);
      this.plugins.sort((a, b) => (a.priority || 100) - (b.priority || 100));

      this.logger.debug(`Registered plugin: ${plugin.name} v${plugin.version}`);
    } catch (error) {
      this.logger.error(`Failed to register plugin ${plugin.name}`, error as Error);
      throw error;
    }
  }

  /**
   * Unregister a plugin
   */
  async unregisterPlugin(pluginName: string): Promise<void> {
    const index = this.plugins.findIndex(p => p.name === pluginName);
    
    if (index === -1) {
      this.logger.warn(`Plugin not found: ${pluginName}`);
      return;
    }

    const plugin = this.plugins[index];
    
    // Call destroy hook
    if (plugin.onDestroy) {
      await plugin.onDestroy();
    }

    this.plugins.splice(index, 1);
    this.logger.debug(`Unregistered plugin: ${pluginName}`);
  }

  /**
   * Execute a hook on all enabled plugins
   */
  private async executeHook(
    hookName: keyof InternalPlugin,
    ...args: any[]
  ): Promise<void> {
    const enabledPlugins = this.plugins.filter(p => p.enabled && p[hookName]);

    for (const plugin of enabledPlugins) {
      try {
        const hook = plugin[hookName] as any;
        await hook.apply(plugin, args);
      } catch (error) {
        this.logger.error(
          `Plugin ${plugin.name} failed during ${String(hookName)}`,
          error as Error
        );
        // Continue with other plugins even if one fails
      }
    }
  }

  /**
   * Lifecycle hooks
   */
  async beforeGeneration(config: any): Promise<void> {
    await this.executeHook('beforeGeneration', config);
  }

  async afterGeneration(metrics: GenerationMetrics): Promise<void> {
    await this.executeHook('afterGeneration', metrics);
  }

  async beforeEntityGeneration(type: string, config: any): Promise<void> {
    await this.executeHook('beforeEntityGeneration', type, config);
  }

  async afterEntityGeneration(context: EntityContext): Promise<void> {
    await this.executeHook('afterEntityGeneration', context);
  }

  async onEntityGenerationError(error: ErrorContext): Promise<void> {
    await this.executeHook('onEntityGenerationError', error);
  }

  /**
   * State management hooks
   */
  onStateChange(state: GeneratorState, changeType: string): void {
    const enabledPlugins = this.plugins.filter(p => p.enabled && p.onStateChange);
    
    for (const plugin of enabledPlugins) {
      try {
        plugin.onStateChange!(state, changeType);
      } catch (error) {
        this.logger.error(
          `Plugin ${plugin.name} failed during onStateChange`,
          error as Error
        );
      }
    }
  }

  async onCheckpoint(checkpointId: string): Promise<void> {
    await this.executeHook('onCheckpoint', checkpointId);
  }

  async onRestore(checkpointId: string): Promise<void> {
    await this.executeHook('onRestore', checkpointId);
  }

  /**
   * Performance monitoring hooks
   */
  onMetricsUpdate(metrics: Partial<GenerationMetrics>): void {
    const enabledPlugins = this.plugins.filter(p => p.enabled && p.onMetricsUpdate);
    
    for (const plugin of enabledPlugins) {
      try {
        plugin.onMetricsUpdate!(metrics);
      } catch (error) {
        this.logger.error(
          `Plugin ${plugin.name} failed during onMetricsUpdate`,
          error as Error
        );
      }
    }
  }

  onMemoryWarning(stats: any): void {
    const enabledPlugins = this.plugins.filter(p => p.enabled && p.onMemoryWarning);
    
    for (const plugin of enabledPlugins) {
      try {
        plugin.onMemoryWarning!(stats);
      } catch (error) {
        this.logger.error(
          `Plugin ${plugin.name} failed during onMemoryWarning`,
          error as Error
        );
      }
    }
  }

  /**
   * Custom event emission
   */
  async emit(eventName: string, data: any): Promise<void> {
    await this.executeHook('onEvent', eventName, data);
  }

  /**
   * Get list of registered plugins
   */
  getPlugins(): Array<{ name: string; version: string; enabled: boolean; priority: number }> {
    return this.plugins.map(p => ({
      name: p.name,
      version: p.version,
      enabled: p.enabled,
      priority: p.priority || 100,
    }));
  }

  /**
   * Enable/disable a plugin
   */
  setPluginEnabled(pluginName: string, enabled: boolean): void {
    const plugin = this.plugins.find(p => p.name === pluginName);
    
    if (plugin) {
      plugin.enabled = enabled;
      this.logger.debug(`Plugin ${pluginName} ${enabled ? 'enabled' : 'disabled'}`);
    } else {
      this.logger.warn(`Plugin not found: ${pluginName}`);
    }
  }

  /**
   * Destroy all plugins and cleanup
   */
  async destroy(): Promise<void> {
    // Destroy in reverse order
    const reversedPlugins = [...this.plugins].reverse();
    
    for (const plugin of reversedPlugins) {
      if (plugin.onDestroy) {
        try {
          await plugin.onDestroy();
        } catch (error) {
          this.logger.error(`Failed to destroy plugin ${plugin.name}`, error as Error);
        }
      }
    }

    this.plugins = [];
    this.initialized = false;
  }
}