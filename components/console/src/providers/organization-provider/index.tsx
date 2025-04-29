'use server'

import React from 'react'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import {
  FetchAllOrganizations,
  FetchAllOrganizationsUseCase
} from '@/core/application/use-cases/organizations/fetch-all-organizations-use-case'
import { serverFetcher } from '@/lib/fetcher'
import { OrganizationProviderClient } from './organization-provider-client'
import { OrganizationResponseDto } from '@/core/application/dto/organization-dto'

const fetchAllOrganizationsUseCase = container.get<FetchAllOrganizations>(
  FetchAllOrganizationsUseCase
)

export const OrganizationProvider = async ({
  children
}: React.PropsWithChildren) => {
  /**
   * TODO: Call the proper get organizations for user
   * For now we setting the first organization as the current one
   */
  const orgResult = await serverFetcher(
    async () => await fetchAllOrganizationsUseCase.execute(10, 1)
  )

  return (
    <OrganizationProviderClient
      organizations={(orgResult?.items as OrganizationResponseDto[]) ?? []}
    >
      {children}
    </OrganizationProviderClient>
  )
}
