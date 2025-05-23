'use client'

import { useRouter } from 'next/navigation'
import { Breadcrumb } from '@/components/breadcrumb'
import { FormDetailsProvider } from '@/context/form-details-context'
import { useIntl } from 'react-intl'
import { OrganizationsForm } from '../organizations-form'
import { PageHeader } from '@/components/page-header'
import useCustomToast from '@/hooks/use-custom-toast'

const Page = () => {
  const intl = useIntl()
  const router = useRouter()
  const { showSuccess } = useCustomToast()

  const handleSuccess = () => {
    showSuccess(
      intl.formatMessage({
        id: 'organizations.toast.create.success',
        defaultMessage: 'Organization created!'
      })
    )
    router.push('/settings')
  }

  return (
    <>
      <Breadcrumb
        paths={[
          {
            name: intl.formatMessage({
              id: 'organizations.organizationView.breadcrumbs.settings',
              defaultMessage: 'Settings'
            }),
            href: `/settings`
          },
          {
            name: intl.formatMessage({
              id: 'organizations.organizationView.breadcrumbs.organizations',
              defaultMessage: 'Organizations'
            }),
            href: `/settings?tab=organizations`
          },
          {
            name: intl.formatMessage({
              id: 'organizations.organizationView.breadcrumbs.newOrganization',
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

      <FormDetailsProvider>
        <OrganizationsForm onSuccess={handleSuccess} />
      </FormDetailsProvider>
    </>
  )
}

export default Page
