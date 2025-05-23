import { StatusDto } from './status.dto'

export interface UpdateAssetDto {
  name?: string
  status?: StatusDto
  metadata?: Record<string, string> | null
}
