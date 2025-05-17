export type IdentityCreateApplicationDto = {
  name: string
  description: string
}

export type IdentityApplicationDto = IdentityCreateApplicationDto & {
  id: string
  clientId: string
  clientSecret: string
  createdAt: Date
}
