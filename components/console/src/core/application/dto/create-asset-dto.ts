import { StatusDto } from './status.dto'

export interface CreateAssetDto {
  name: string
  type: string
  code: string
  status: StatusDto
  metadata: Record<string, string> | null
}
