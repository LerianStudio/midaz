import { createContext, PropsWithChildren, useContext, useState } from 'react'
import { useStepper } from '@/hooks/use-stepper'
import { FormData } from './schemas'
import { useRouter } from 'next/navigation'
import { createQueryString } from '@/lib/search'
import { OrganizationDto } from '@/core/application/dto/organization-dto'
import { useOrganization } from '@/providers/organization-provider'
import { useCreateOnboardingOrganization } from '@/client/onboarding'

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

  const [data, _setData] = useState<FormData>({} as FormData)
  const stepper = useStepper({ maxSteps: 3 })

  const { mutate: createOrganization, isPending: loading } =
    useCreateOnboardingOrganization({
      onSuccess: (organization: OrganizationDto) => {
        setOrganization(organization)
        router.push(`/onboarding/ledger` + createQueryString({ process: true }))
      }
    })

  const handleCancel = () => router.push('/onboarding')

  const handleSubmit = (values: Partial<FormData> = {} as FormData) => {
    const newData = { ...data, ...values } as FormData
    setData(newData)
    createOrganization(newData as any)
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
