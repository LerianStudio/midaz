'use client'

import React from 'react'
import Image from 'next/image'
import LoadingScreen from '@/components/loading-screen'
import LerianLogo from '@/images/lerian-logo-outline.webp'
import BackgroundImage from '@/images/bg-wallpaper.webp'
import { Form } from '@/components/ui/form'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { useRouter } from 'next/navigation'
import { zodResolver } from '@hookform/resolvers/zod'
import { signIn } from 'next-auth/react'
import { auth } from '@/schema/auth'
import { useIntl } from 'react-intl'
import { InputField } from '@/components/form'
import { LoadingButton } from '@/components/ui/loading-button'
import { ArrowRight } from 'lucide-react'
import { useListOrganizations } from '@/client/organizations'
import { useToast } from '@/hooks/use-toast'

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
  const { toast } = useToast()
  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    defaultValues
  })

  const [isLoading, setIsLoading] = React.useState(false)
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
    const result = await signIn('credentials', {
      ...values,
      redirect: false
    })

    if (result?.error) {
      console.error('Login error ->', result)
      toast({
        description: intl.formatMessage({
          id: 'signIn.toast.error',
          defaultMessage: 'Invalid credentials.'
        }),
        variant: 'destructive'
      })
      return
    }

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

          <p className="text-shadcn-400 pt-4 text-sm">
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
                method="post"
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

        <div className="relative z-10">
          <Image alt="Lerian Logo" src={LerianLogo} width={240} height={240} />
        </div>
      </div>
    </div>
  )
}

export default SignInPage
