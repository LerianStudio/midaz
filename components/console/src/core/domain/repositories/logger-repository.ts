import { LogContext, LogMetadata } from '@/core/domain/entities/log-entities'

export abstract class LoggerRepository {
  abstract info(
    message: string,
    context?: LogContext,
    metadata?: LogMetadata
  ): void
  abstract error(
    message: string,
    context?: LogContext,
    metadata?: LogMetadata
  ): void
  abstract warn(
    message: string,
    context?: LogContext,
    metadata?: LogMetadata
  ): void
  abstract debug(
    message: string,
    context?: LogContext,
    metadata?: LogMetadata
  ): void
  abstract audit(
    message: string,
    context?: LogContext,
    metadata?: LogMetadata
  ): void
}
