import { createContext, PropsWithChildren, useContext, useState } from 'react'
import { useStepper } from '@/hooks/use-stepper'
import { FormData } from './schemas'
import { useRouter } from 'next/navigation'
import { omit } from 'lodash'
import { createQueryString } from '@/lib/search'
import { OrganizationsType } from '@/types/organizations-type'
import { useOrganization } from '@/context/organization-provider/organization-provider-client'
import { useCreateOnboardingOrganization } from '@/client/onboarding'
import useCustomToast from '@/hooks/use-custom-toast'

type OnboardFormContextProps = ReturnType<typeof useStepper> & {
  data: FormData
  setData: (values: Partial<FormData>) => void
  loading: boolean
  handleCancel: () => void
  handleSubmit: (values?: Partial<FormData>) => void
}

const OnboardFormContext = createContext<OnboardFormContextProps>(
  {} as OnboardFormContextProps
)

export function useOnboardForm() {
  return useContext(OnboardFormContext)
}

export type OnboardFormProviderProps = PropsWithChildren

export function OnboardFormProvider({ children }: OnboardFormProviderProps) {
  const router = useRouter()
  const { setOrganization } = useOrganization()
  const { showError } = useCustomToast()

  const [data, _setData] = useState<FormData>({} as FormData)
  const stepper = useStepper({ maxSteps: 3 })

  const { mutate: createOrganization, isPending: loading } =
    useCreateOnboardingOrganization({
      onSuccess: (organization: OrganizationsType) => {
        setOrganization(organization)
        router.push(`/onboarding/ledger` + createQueryString({ process: true }))
      },
      onError: (error: any) => {
        showError(error.message)
      }
    })

  const parse = (data: FormData) => ({
    ...omit(data, ['accentColor', 'avatar']),
    metadata: {
      ...(data?.accentColor ? { accentColor: data?.accentColor } : {}),
      ...(data?.avatar ? { avatar: data?.avatar } : {})
    }
  })

  const handleCancel = () => router.push('/onboarding')

  const handleSubmit = (values: Partial<FormData> = {} as FormData) => {
    const newData = { ...data, ...values } as FormData
    setData(newData)
    createOrganization(parse(newData) as any)
  }

  const setData = (values: Partial<FormData>) =>
    _setData((prev) => ({ ...prev, ...values }))

  return (
    <OnboardFormContext.Provider
      value={{
        ...stepper,
        data,
        setData,
        loading,
        handleCancel,
        handleSubmit
      }}
    >
      {children}
    </OnboardFormContext.Provider>
  )
}
