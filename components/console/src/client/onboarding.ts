import { useLayoutQueryClient } from '@lerianstudio/console-layout'
import { OrganizationDto } from '@/core/application/dto/organization-dto'
import { postFetcher } from '@/lib/fetcher'
import {
  useMutation,
  UseMutationOptions,
  useQueryClient
} from '@tanstack/react-query'

export const useCreateOnboardingOrganization = ({ ...options }) => {
  return useMutation<OrganizationDto>({
    mutationKey: ['onboarding'],
    mutationFn: postFetcher(`/api/onboarding`),
    ...options
  })
}

type UseCompleteOnboardingProps = UseMutationOptions & {
  organizationId: string
}

export const useCompleteOnboarding = ({
  organizationId,
  onSuccess,
  ...options
}: UseCompleteOnboardingProps) => {
  const queryClient = useQueryClient()
  const layoutQueryClient = useLayoutQueryClient()

  return useMutation<any, any, any>({
    mutationKey: ['onboarding', organizationId, 'complete'],
    mutationFn: postFetcher(`/api/onboarding/${organizationId}/complete`),
    ...options,
    onSuccess: (...args) => {
      queryClient.invalidateQueries({ queryKey: ['organizations'] })
      layoutQueryClient.invalidateQueries({ queryKey: ['organizations'] })
      layoutQueryClient.invalidateQueries({
        queryKey: ['ledgers', organizationId]
      })
      onSuccess?.(...args)
    }
  })
}
