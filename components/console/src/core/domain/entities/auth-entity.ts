export interface AuthEntity {
  username: string
  password: string
}

export interface AuthResponseEntity {
  accessToken: string
  refreshToken: string
  idToken: string
  tokenType: string
  expiresIn: number
  scope: string
}

export interface AuthSessionEntity {
  id: string
  username: string
  name: string
  accessToken: string
  refreshToken: string
  idToken: string
}
