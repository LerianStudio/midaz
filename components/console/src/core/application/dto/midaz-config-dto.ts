export type MidazConfigDto = {
  isConfigEnabled: boolean
  config: Array<{
    organization: string
    ledgers: string[]
  }>
}
