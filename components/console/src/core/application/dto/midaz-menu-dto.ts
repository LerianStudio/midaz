export type MidazMenuDto = {
  groups: MidazMenuGroupDto[]
}

export type MidazMenuGroupDto = {
  id: string
  title?: string
  showSeparatorAfter: boolean
  items: MidazMenuItemDto[]
}

export type MidazMenuItemDto = {
  name: string
  title: string
  host: string
  route: string
  icon: string
  hasLedgerDependencies: boolean
  order: number
}
