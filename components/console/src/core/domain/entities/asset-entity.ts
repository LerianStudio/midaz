export type AssetEntity = {
  id?: string
  organizationId?: string
  ledgerId?: string
  name: string
  type: string
  code: string
  metadata: Record<string, string> | null
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date | null
}
