import {
  AuthEntity,
  AuthSessionEntity
} from '@/core/domain/entities/auth-entity'
import { AuthLoginRepository } from '@/core/domain/repositories/auth/auth-login-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import type { AuthLoginDto, AuthSessionDto } from '../../dto/auth-dto'
import { AuthMapper } from '../../mappers/auth-mapper'

export interface AuthLogin {
  execute: (loginData: AuthLoginDto) => Promise<AuthSessionDto>
}

@injectable()
export class AuthLoginUseCase implements AuthLogin {
  constructor(
    @inject(AuthLoginRepository)
    private readonly authLoginRepository: AuthLoginRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(loginData: AuthLoginDto): Promise<AuthSessionDto> {
    const authLoginEntity: AuthEntity = AuthMapper.toDomain(loginData)

    const authLoginResponse: AuthSessionEntity =
      await this.authLoginRepository.login(authLoginEntity)

    const authLoginResponseDto: AuthSessionDto =
      AuthMapper.toDto(authLoginResponse)

    return authLoginResponseDto
  }
}
