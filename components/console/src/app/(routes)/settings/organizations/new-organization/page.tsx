'use client'

import { useRouter } from 'next/navigation'
import { Breadcrumb } from '@/components/breadcrumb'
import { useIntl } from 'react-intl'
import { OrganizationsForm } from '../organizations-form'
import { PageHeader } from '@/components/page-header'
import { useToast } from '@/hooks/use-toast'

const Page = () => {
  const intl = useIntl()
  const router = useRouter()
  const { toast } = useToast()

  const handleSuccess = () => {
    toast({
      description: intl.formatMessage({
        id: 'success.organizations.create',
        defaultMessage: 'Organization created!'
      }),
      variant: 'success'
    })
    router.push('/settings')
  }

  return (
    <>
      <Breadcrumb
        paths={[
          {
            name: intl.formatMessage({
              id: 'settings.title',
              defaultMessage: 'Settings'
            }),
            href: `/settings`
          },
          {
            name: intl.formatMessage({
              id: 'organizations.title',
              defaultMessage: 'Organizations'
            }),
            href: `/settings?tab=organizations`
          },
          {
            name: intl.formatMessage({
              id: 'organizations.organizationView.newOrganization.title',
              defaultMessage: 'New Organization'
            })
          }
        ]}
      />
      <PageHeader.Root>
        <PageHeader.Wrapper className="border-none">
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'organizations.organizationView.newOrganization.title',
              defaultMessage: 'New Organization'
            })}
          />
        </PageHeader.Wrapper>
      </PageHeader.Root>

      <OrganizationsForm onSuccess={handleSuccess} />
    </>
  )
}

export default Page
