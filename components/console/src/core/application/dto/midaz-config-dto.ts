export type MidazConfigDto = {
  isConfigEnabled: boolean
  config: Array<{
    organization: string
    ledgers: string[]
  }>
}

export type MidazConfigValidationDto = {
  isConfigEnabled: boolean
  isLedgerAllowed: boolean
}
