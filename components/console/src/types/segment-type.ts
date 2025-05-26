import { Metadata } from './metadata-type'

export type SegmentType = {
  id: string
  name: string
  metadata: Metadata
  organizationId: string
  ledgerId: string
}
