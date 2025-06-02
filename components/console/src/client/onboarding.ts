import { postFetcher } from '@/lib/fetcher'
import { OrganizationsType } from '@/types/organizations-type'
import {
  useMutation,
  UseMutationOptions,
  useQueryClient
} from '@tanstack/react-query'

export const useCreateOnboardingOrganization = ({ ...options }) => {
  return useMutation<OrganizationsType>({
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

  return useMutation<any, any, any>({
    mutationKey: ['onboarding', organizationId, 'complete'],
    mutationFn: postFetcher(`/api/onboarding/${organizationId}/complete`),
    ...options,
    onSuccess: (...args) => {
      queryClient.invalidateQueries({ queryKey: ['organizations'] })
      onSuccess?.(...args)
    }
  })
}
