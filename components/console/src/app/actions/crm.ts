'use server'

import { revalidatePath } from 'next/cache'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { HolderRepository } from '@/core/domain/repositories/crm/holder-repository'
import { AliasRepository } from '@/core/domain/repositories/crm/alias-repository'
import { CRM_SYMBOLS } from '@/core/infrastructure/container-registry/midaz-plugins/crm-module'
import {
  CreateHolderUseCase,
  CreateHolder
} from '@/core/application/use-cases/crm/holders/create-holder-use-case'
import {
  FetchAllHoldersUseCase,
  FetchAllHolders
} from '@/core/application/use-cases/crm/holders/fetch-all-holders-use-case'
import {
  FetchHolderByIdUseCase,
  FetchHolderById
} from '@/core/application/use-cases/crm/holders/fetch-holder-by-id-use-case'
import {
  UpdateHolderUseCase,
  UpdateHolder
} from '@/core/application/use-cases/crm/holders/update-holder-use-case'
import {
  DeleteHolderUseCase,
  DeleteHolder
} from '@/core/application/use-cases/crm/holders/delete-holder-use-case'
import {
  CreateAliasUseCase,
  CreateAlias
} from '@/core/application/use-cases/crm/aliases/create-alias-use-case'
import {
  FetchAllAliasesUseCase,
  FetchAllAliases
} from '@/core/application/use-cases/crm/aliases/fetch-all-aliases-use-case'
import {
  HolderEntity,
  CreateHolderEntity,
  UpdateHolderEntity
} from '@/core/domain/entities/holder-entity'
import {
  AliasEntity,
  CreateAliasEntity,
  UpdateAliasEntity
} from '@/core/domain/entities/alias-entity'

interface ActionResult<T> {
  success: boolean
  data?: T
  error?: string
}

function handleError(
  error: unknown,
  defaultMessage: string
): ActionResult<any> {
  console.error(defaultMessage, error)

  if (error instanceof Error) {
    if (
      error.message.includes('503') ||
      error.message.includes('Service Unavailable')
    ) {
      return {
        success: false,
        error: 'CRM service is currently unavailable. Please try again later.'
      }
    }
    return {
      success: false,
      error: error.message
    }
  }

  return {
    success: false,
    error: defaultMessage
  }
}

// Holder Actions
export async function getHolders(params?: {
  organizationId?: string
  limit?: number
  page?: number
}): Promise<ActionResult<{ holders: HolderEntity[]; total: number }>> {
  try {
    const { organizationId = 'default', limit = 10, page = 1 } = params || {}

    const holderRepository = container.get<HolderRepository>(
      CRM_SYMBOLS.HolderRepository
    )
    const result = await holderRepository.fetchAll(organizationId, limit, page)

    return {
      success: true,
      data: {
        holders: result.items,
        total: result.total || 0
      }
    }
  } catch (error) {
    return handleError(error, 'Failed to fetch holders')
  }
}

export async function getHolderById(
  id: string
): Promise<ActionResult<HolderEntity>> {
  try {
    const holderRepository = container.get<HolderRepository>(
      CRM_SYMBOLS.HolderRepository
    )
    const holder = await holderRepository.findById(id)

    return {
      success: true,
      data: holder
    }
  } catch (error) {
    return handleError(error, 'Failed to fetch holder')
  }
}

export async function createHolder(
  holder: CreateHolderEntity
): Promise<ActionResult<HolderEntity>> {
  try {
    const createHolderUseCase = container.get<CreateHolder>(CreateHolderUseCase)
    const created = await createHolderUseCase.execute('default', holder)

    revalidatePath('/plugins/crm/holders')

    return {
      success: true,
      data: created
    }
  } catch (error) {
    return handleError(error, 'Failed to create holder')
  }
}

export async function updateHolder(
  id: string,
  updates: UpdateHolderEntity
): Promise<ActionResult<HolderEntity>> {
  try {
    const updateHolderUseCase = container.get<UpdateHolder>(UpdateHolderUseCase)
    const updated = await updateHolderUseCase.execute('default', id, updates)

    revalidatePath('/plugins/crm/holders')
    revalidatePath(`/plugins/crm/holders/${id}`)

    return {
      success: true,
      data: updated
    }
  } catch (error) {
    return handleError(error, 'Failed to update holder')
  }
}

export async function deleteHolder(
  id: string,
  isHardDelete: boolean = false
): Promise<ActionResult<void>> {
  try {
    const deleteHolderUseCase = container.get<DeleteHolder>(DeleteHolderUseCase)
    await deleteHolderUseCase.execute('default', id, isHardDelete)

    revalidatePath('/plugins/crm/holders')

    return {
      success: true
    }
  } catch (error) {
    return handleError(error, 'Failed to delete holder')
  }
}

// Alias Actions
export async function getAliasesByHolderId(params: {
  holderId: string
  organizationId?: string
  limit?: number
  page?: number
}): Promise<ActionResult<{ aliases: AliasEntity[]; total: number }>> {
  try {
    const {
      holderId,
      organizationId = 'default',
      limit = 10,
      page = 1
    } = params

    const aliasRepository = container.get<AliasRepository>(
      CRM_SYMBOLS.AliasRepository
    )
    const result = await aliasRepository.fetchAllByHolder(
      holderId,
      organizationId,
      limit,
      page
    )

    return {
      success: true,
      data: {
        aliases: result.items,
        total: result.total || 0
      }
    }
  } catch (error) {
    return handleError(error, 'Failed to fetch aliases')
  }
}

export async function getAllAliases(params?: {
  organizationId?: string
  limit?: number
  page?: number
}): Promise<ActionResult<{ aliases: AliasEntity[]; total: number }>> {
  try {
    const { organizationId = 'default', limit = 10, page = 1 } = params || {}

    const fetchAllAliasesUseCase = container.get<FetchAllAliases>(
      FetchAllAliasesUseCase
    )
    const result = await fetchAllAliasesUseCase.execute(
      organizationId,
      '', // TODO: This should be a different use case to fetch all aliases
      limit,
      page
    )

    return {
      success: true,
      data: {
        aliases: result.items,
        total: result.total || 0
      }
    }
  } catch (error) {
    return handleError(error, 'Failed to fetch all aliases')
  }
}

export async function createAlias(
  holderId: string,
  alias: CreateAliasEntity
): Promise<ActionResult<AliasEntity>> {
  try {
    const createAliasUseCase = container.get<CreateAlias>(CreateAliasUseCase)
    const created = await createAliasUseCase.execute('default', holderId, alias)

    revalidatePath('/plugins/crm/holders')
    revalidatePath(`/plugins/crm/holders/${holderId}`)
    revalidatePath('/plugins/crm/aliases')

    return {
      success: true,
      data: created
    }
  } catch (error) {
    return handleError(error, 'Failed to create alias')
  }
}

export async function updateAlias(
  holderId: string,
  aliasId: string,
  updates: UpdateAliasEntity
): Promise<ActionResult<AliasEntity>> {
  try {
    const aliasRepository = container.get<AliasRepository>(
      CRM_SYMBOLS.AliasRepository
    )
    const updated = await aliasRepository.update(holderId, aliasId, updates)

    revalidatePath('/plugins/crm/holders')
    revalidatePath(`/plugins/crm/holders/${holderId}`)
    revalidatePath('/plugins/crm/aliases')

    return {
      success: true,
      data: updated
    }
  } catch (error) {
    return handleError(error, 'Failed to update alias')
  }
}

export async function deleteAlias(
  holderId: string,
  aliasId: string,
  isHardDelete: boolean = false
): Promise<ActionResult<void>> {
  try {
    const aliasRepository = container.get<AliasRepository>(
      CRM_SYMBOLS.AliasRepository
    )
    await aliasRepository.delete(holderId, aliasId, isHardDelete)

    revalidatePath('/plugins/crm/holders')
    revalidatePath(`/plugins/crm/holders/${holderId}`)
    revalidatePath('/plugins/crm/aliases')

    return {
      success: true
    }
  } catch (error) {
    return handleError(error, 'Failed to delete alias')
  }
}
