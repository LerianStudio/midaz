export interface LogMetadata {
  userId?: string
  organizationId?: string
  [key: string]: any
}

export interface LogContext {
  events?: Record<string, any>
}

export interface LogEntry {
  level: 'INFO' | 'ERROR' | 'WARN' | 'DEBUG' | 'AUDIT'
  message: string
  timestamp: string
  midazId?: string
  metadata?: LogMetadata
  context?: LogContext
}
