import { v4 as uuidv4 } from 'uuid'
import { injectable } from 'inversify'

/**
 * Class responsible for managing a unique ID per request
 * Uses the @injectable() decorator from InversifyJS for dependency injection
 */
@injectable()
export class MidazRequestContext {
  // Stores the unique ID for the current request. Initially null
  private requestScopedId: string | null = null

  /**
   * Returns the unique ID for the current request
   * If no ID exists, generates a new one using UUID v4
   */
  getMidazId(): string {
    if (!this.requestScopedId) {
      this.requestScopedId = uuidv4()
    }
    return this.requestScopedId
  }

  /**
   * Resets the ID between requests
   * Used to ensure each request gets a new unique identifier
   */
  clearMidazId(): void {
    this.requestScopedId = null
  }
}
