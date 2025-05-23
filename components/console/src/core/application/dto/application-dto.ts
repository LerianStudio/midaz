export type CreateApplicationDto = {
  name: string
  description: string
}

export type ApplicationResponseDto = {
  id: string
  name: string
  description: string
  clientId: string
  clientSecret: string
  createdAt: Date
}
