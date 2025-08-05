export type AuthLoginDto = {
  username: string
  password: string
}

export type AuthLoginResponseDto = {
  access_token: string
  refresh_token: string
  token_type: string
  expires_in: number
  scope: string
}

export type AuthSessionDto = {
  id: string
  username: string
  name: string
  access_token: string
  refresh_token: string
}

type AuthResourceDto = string
type AuthActionDto = string

export type AuthPermissionDto = Record<AuthResourceDto, AuthActionDto[]>
