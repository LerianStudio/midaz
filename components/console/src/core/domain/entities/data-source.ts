export interface DataSource {
  id: string
  name: string
  type: DataSourceType
  description?: string
  status: DataSourceStatus
  connectionString?: string
  tables: DataSourceTable[]
  lastSync?: string
  queryCount: number
  metadata: Record<string, any>
  createdAt: string
  updatedAt: string
}

export type DataSourceType = 'postgresql' | 'mongodb' | 'mysql' | 'api' | 'file'
export type DataSourceStatus =
  | 'connected'
  | 'disconnected'
  | 'error'
  | 'syncing'

export interface DataSourceTable {
  name: string
  fields: string[]
  recordCount?: number
  lastUpdated?: string
}

export interface DataSourceConnection {
  host: string
  port: number
  database: string
  username: string
  password: string
  ssl?: boolean
  timeout?: number
}

export interface CreateDataSourceInput {
  name: string
  type: DataSourceType
  description?: string
  connection: DataSourceConnection
  metadata?: Record<string, any>
}

export interface UpdateDataSourceInput {
  name?: string
  description?: string
  connection?: Partial<DataSourceConnection>
  metadata?: Record<string, any>
}

export interface DataSourceHealthCheck {
  dataSourceId: string
  status: DataSourceStatus
  responseTime?: number
  lastChecked: string
  error?: string
  details?: Record<string, any>
}

export interface DataSourceQuery {
  dataSourceId: string
  query: string
  parameters?: Record<string, any>
  limit?: number
  offset?: number
}

export interface DataSourceQueryResult {
  columns: string[]
  rows: any[][]
  totalCount?: number
  executionTime: number
}
