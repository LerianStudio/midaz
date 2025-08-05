import { LoggerAggregator } from '@lerianstudio/lib-logs'
import { inject } from 'inversify'

export abstract class BaseController {
  @inject(LoggerAggregator)
  protected readonly logger!: LoggerAggregator
}
