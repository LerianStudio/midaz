import { StatusDto } from './status.dto'

export interface UpdateLedgerDto {
  name?: string
  status?: StatusDto
  metadata?: Record<string, any>
}
