'use client'

import { use } from 'react'
import { useRouter } from 'next/navigation'
import { Breadcrumb } from '@/components/breadcrumb'
import { PageHeader } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { useIntl } from 'react-intl'
import { useGetOrganization } from '@/client/organizations'
import { OrganizationsForm } from '../organizations-form'
import { NotFoundContent } from '@/components/not-found-content'
import { useToast } from '@/hooks/use-toast'

const Page = (props: { params: Promise<{ id: string }> }) => {
  const params = use(props.params)
  const router = useRouter()
  const intl = useIntl()
  const organizationId = params.id

  const { data, error, isPending } = useGetOrganization({
    organizationId
  })
  const { toast } = useToast()

  const handleSuccess = () => {
    toast({
      description: intl.formatMessage({
        id: 'success.organizations.update',
        defaultMessage: 'Organization updated successfully!'
      }),
      variant: 'success'
    })
    router.push('/settings')
  }

  if (error && !isPending) {
    return (
      <NotFoundContent
        title={intl.formatMessage({
          id: 'organizations.organizationView.notFound',
          defaultMessage: 'Organization not found.'
        })}
      />
    )
  }

  if (isPending) {
    return <Skeleton className="h-screen bg-zinc-200" />
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
            name: params.id
          }
        ]}
      />

      <PageHeader.Root>
        <PageHeader.Wrapper className="border-none">
          <PageHeader.InfoTitle
            title={data.legalName}
            subtitle={organizationId}
          >
            <PageHeader.InfoTooltip subtitle={data.id} />
          </PageHeader.InfoTitle>
        </PageHeader.Wrapper>
      </PageHeader.Root>

      <OrganizationsForm data={data} onSuccess={handleSuccess} />
    </>
  )
}

export default Page
