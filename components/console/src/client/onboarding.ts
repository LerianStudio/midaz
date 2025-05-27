import { postFetcher } from '@/lib/fetcher'
import { OrganizationsType } from '@/types/organizations-type'
import { useMutation, UseMutationOptions } from '@tanstack/react-query'

export const useCreateOnboardingOrganization = ({ ...options }: UseMutationOptions<OrganizationsType, unknown, any> = {}) => {
  return useMutation<OrganizationsType, unknown, any>({
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
  ...options
}: UseCompleteOnboardingProps) => {
  return useMutation<any, any, any>({
    mutationKey: ['onboarding', organizationId, 'complete'],
    mutationFn: postFetcher(`/api/onboarding/${organizationId}/complete`),
    ...options
  })
}
