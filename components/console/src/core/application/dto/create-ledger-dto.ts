import { StatusDto } from './status.dto'

export interface CreateLedgerDto {
  name: string
  status: StatusDto
  metadata: Record<string, any>
}
