import { BalanceEntity } from '@/core/domain/entities/balance-entity'

export class BalanceMapper {
  public static toDomain(dto: any): BalanceEntity {
    return {
      allowSending: dto.allowSending!,
      allowReceiving: dto.allowReceiving!
    }
  }

  public static toResponseDto(entity: any): any {
    return {
      allowSending: entity.allowSending,
      allowReceiving: entity.allowReceiving
    }
  }
}
