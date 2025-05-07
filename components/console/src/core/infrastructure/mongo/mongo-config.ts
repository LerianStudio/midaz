/**
 * @file MongoDB Configuration
 * @description Provides database configuration and connection management for MongoDB
 * using mongoose. Implements dependency injection with inversify and supports
 * internationalization for error messages.
 */

import { inject, injectable } from 'inversify'
import mongoose, { Mongoose } from 'mongoose'
import { LoggerAggregator } from '../logger/logger-aggregator'
import { getIntl } from '@/lib/intl'
import { DatabaseException } from './exceptions/database-exception'
import { IntlShape } from 'react-intl'

/**
 * Abstract database configuration class
 * @abstract
 * @template T - The database client type
 * @description Defines the contract for database configurations across different database providers.
 * This abstraction allows for potential future support of different database systems.
 */
export abstract class DBConfig<T> {
  /**
   * Establishes a connection to the database
   * @param params - Connection parameters including URI, database name, and credentials
   * @returns A promise that resolves when the connection is established
   */
  abstract connect(params: DBConfigParams): Promise<void>

  /**
   * Retrieves the database client instance
   * @returns A promise that resolves with the database client
   */
  abstract getClient(): Promise<T>
}

/**
 * Database configuration parameters interface
 * @interface
 * @description Defines the required parameters for connecting to a database
 */
export interface DBConfigParams {
  uri: string
  dbName: string
  user: string
  pass: string
}

/**
 * MongoDB configuration implementation
 * @class
 * @implements {DBConfig<Mongoose>}
 * @description Manages MongoDB connections via mongoose. Handles connection lifecycle,
 * error handling, and provides access to the mongoose client.
 */
@injectable()
export class MongoConfig implements DBConfig<Mongoose> {
  private intl?: IntlShape

  /**
   * Creates a new MongoDB configuration instance
   * @param logger - Injected logger for connection events and error tracking
   */
  constructor(
    @inject(LoggerAggregator) private readonly logger: LoggerAggregator
  ) {}

  /**
   * Connects to MongoDB using the provided parameters
   * @param params - Connection parameters including URI, database name, and credentials
   * @throws {DatabaseException} If connection fails or if connection state is invalid after connect
   */
  async connect({ uri, dbName, user, pass }: DBConfigParams) {
    const intl = await this.getIntlSafe()

    if (this.isConnected()) {
      return
    }

    this.logger.info('[MONGO] Connecting...')

    try {
      await mongoose.connect(uri, {
        dbName,
        user,
        pass
      })

      if (!this.isConnected()) {
        throw new DatabaseException(this.getUnexpectedDatabaseMessage())
      }

      this.logger.info('[MONGO] Connected to MongoDB')
    } catch (error) {
      this.logger.error('[MONGO] Failed to connect to MongoDB', error)

      throw new DatabaseException(this.getUnexpectedDatabaseMessage())
    }
  }

  /**
   * Retrieves the mongoose client instance for direct database operations
   * @returns The mongoose instance
   * @throws {DatabaseException} If not connected to the database
   */
  async getClient(): Promise<Mongoose> {
    if (!this.intl) {
      this.intl = await getIntl()
    }

    if (!this.isConnected()) {
      throw new DatabaseException(this.getUnexpectedDatabaseMessage())
    }

    return mongoose
  }

  /**
   * Checks if the mongoose connection is currently established
   * @returns True if connected, false otherwise
   * @private
   */
  private isConnected(): boolean {
    return mongoose.connection.readyState === 1
  }

  /**
   * Safely retrieves the internationalization service, initializing it if needed
   * @returns The internationalization service
   * @private
   */
  private async getIntlSafe(): Promise<IntlShape> {
    if (!this.intl) {
      this.intl = await getIntl()
    }
    return this.intl
  }

  /**
   * Gets a localized unexpected database error message
   * @returns Localized error message string
   * @private
   */
  private getUnexpectedDatabaseMessage(): string {
    return this.intl!.formatMessage({
      id: 'error.database.unexpected',
      defaultMessage: 'Unexpected error'
    })
  }
}
