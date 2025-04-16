import useCustomToast from '@/hooks/use-custom-toast'
import { postFetcher } from '@/lib/fetcher'
import { OrganizationsType } from '@/types/organizations-type'
import { useMutation, UseMutationOptions } from '@tanstack/react-query'

export const useCreateOnboardingOrganization = ({ ...options }) => {
  const { showError } = useCustomToast()

  return useMutation<OrganizationsType>({
    mutationKey: ['onboarding'],
    mutationFn: postFetcher(`/api/onboarding`),
    ...options,
    onError: (error) => {
      showError(error.message)
      options.onError?.(error)
    }
  })
}

type UseCompleteOnboardingProps = UseMutationOptions & {
  organizationId: string
  onError: (error: any) => void
}

export const useCompleteOnboarding = ({
  organizationId,
  ...options
}: UseCompleteOnboardingProps) => {
  const { showError } = useCustomToast()

  return useMutation<any, any, any>({
    mutationKey: ['onboarding', organizationId, 'complete'],
    mutationFn: postFetcher(`/api/onboarding/${organizationId}/complete`),
    ...options,
    onError: (error) => {
      showError(error.message)
      options.onError?.(error)
    }
  })
}
