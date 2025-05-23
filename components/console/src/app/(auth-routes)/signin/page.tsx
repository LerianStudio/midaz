'use client'

import Image from 'next/image'
import { Form } from '@/components/ui/form'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { useRouter } from 'next/navigation'
import { zodResolver } from '@hookform/resolvers/zod'
import useCustomToast from '@/hooks/use-custom-toast'
import { signIn } from 'next-auth/react'
import { auth } from '@/schema/auth'
import { useIntl } from 'react-intl'
import { InputField } from '@/components/form'
import { LoadingButton } from '@/components/ui/loading-button'
import { ArrowRight } from 'lucide-react'
import React from 'react'
import LoadingScreen from '@/components/loading-screen'
import MidazLogo from '@/images/midaz-login-screen.webp'
import BackgroundImage from '@/images/login-wallpaper.webp'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import { useListOrganizations } from '@/client/organizations'

const FormSchema = z.object({
  username: auth.username,
  password: auth.password
})

type FormData = z.infer<typeof FormSchema>

const defaultValues = {
  username: '',
  password: ''
}

const SignInPage = () => {
  const intl = useIntl()
  const route = useRouter()
  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    defaultValues
  })

  const { showError } = useCustomToast()
  const [isLoading, setIsLoading] = React.useState(false)
  const [isSubmitting, setIsSubmitting] = React.useState(false)
  const [redirectUrl, setRedirectUrl] = React.useState<string | null>(null)

  const { data: organizationsData, isLoading: orgLoading } =
    useListOrganizations({
      enabled: isLoading
    })

  React.useEffect(() => {
    if (isLoading && !orgLoading) {
      if (organizationsData?.items && organizationsData.items.length > 0) {
        setRedirectUrl('/')
      } else {
        setRedirectUrl('/onboarding')
      }
    }
  }, [isLoading, orgLoading, organizationsData])

  const onSubmit = async (values: FormData) => {
    setIsSubmitting(true)

    const result = await signIn('credentials', {
      ...values,
      redirect: false
    })

    if (result?.error) {
      console.error('Login error ->', result)
      showError(
        intl.formatMessage({
          id: 'signIn.toast.error',
          defaultMessage: 'Invalid credentials.'
        })
      )
      setIsSubmitting(false)
      return
    }

    setIsSubmitting(false)
    setIsLoading(true)
  }

  if (isLoading) {
    return (
      <LoadingScreen
        onComplete={() => {
          if (redirectUrl) {
            route.replace(redirectUrl)
          }
        }}
      />
    )
  }

  return (
    <div className="flex h-screen w-screen">
      <div className="flex h-screen w-3/6 items-center justify-center p-8">
        <div className="w-full max-w-[440px] border-none px-8 shadow-none">
          <h1 className="text-4xl font-bold">
            {intl.formatMessage({
              id: 'signIn.titleLogin',
              defaultMessage: 'Welcome back!'
            })}
          </h1>

          <p className="pt-4 text-sm text-shadcn-400">
            {intl.formatMessage({
              id: 'signIn.descriptionLogin',
              defaultMessage: 'Enter your email and password to continue.'
            })}
          </p>

          <div className="pt-8">
            <Form {...form}>
              <form
                onSubmit={form.handleSubmit(onSubmit)}
                className="space-y-8"
              >
                <InputField
                  control={form.control}
                  type="email"
                  name="username"
                  label={intl.formatMessage({
                    id: 'common.email',
                    defaultMessage: 'E-mail'
                  })}
                  placeholder={intl.formatMessage({
                    id: 'signIn.placeholderEmail',
                    defaultMessage: 'Enter your registered email...'
                  })}
                />

                <InputField
                  control={form.control}
                  name="password"
                  label={intl.formatMessage({
                    id: 'common.password',
                    defaultMessage: 'Password'
                  })}
                  type="password"
                  placeholder={intl.formatMessage({
                    id: 'signIn.placeholderPassword',
                    defaultMessage: '******'
                  })}
                  labelExtra={
                    <TooltipProvider>
                      <Tooltip delayDuration={300}>
                        <TooltipTrigger>
                          <span className="cursor-pointer text-sm font-medium text-slate-900 underline">
                            {intl.formatMessage({
                              id: 'entity.auth.reset.password',
                              defaultMessage: 'I forgot the password'
                            })}
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>
                          <p>
                            {intl.formatMessage({
                              id: 'tooltip.passwordInfo',
                              defaultMessage: 'Contact the system administrator'
                            })}
                          </p>
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  }
                />

                <LoadingButton
                  className="w-full"
                  type="submit"
                  loading={form.formState.isSubmitting}
                  size="xl"
                  icon={<ArrowRight />}
                  iconPlacement="far-end"
                >
                  {intl.formatMessage({
                    id: 'signIn.buttonSignIn',
                    defaultMessage: 'Continue'
                  })}
                </LoadingButton>
              </form>
            </Form>
          </div>
        </div>
      </div>

      <div className="relative flex w-3/6 items-center justify-center">
        <Image
          alt="Login background image"
          src={BackgroundImage}
          fill
          sizes="50vw, 100vh"
          className="object-cover"
        />
        <div className="absolute inset-0 bg-black bg-opacity-50"></div>
        <div className="relative z-10">
          <Image alt="Midaz Logo" src={MidazLogo} width={150} height={150} />
        </div>
      </div>
    </div>
  )
}

export default SignInPage
