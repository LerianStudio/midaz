export type InstrumentsEntity = {
  id: string
  ledgerId: string
  name: string
  code: string
  metadata: {
    internalExchangeAddress: string
    internalExchangeCustody: string
  }
}
