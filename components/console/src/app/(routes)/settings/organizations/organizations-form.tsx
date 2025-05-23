'use client'

import React from 'react'
import type { OrganizationsType } from '@/types/organizations-type'
import { Card } from '@/components/card'
import { Separator } from '@/components/ui/separator'
import { CardContent, CardFooter } from '@/components/ui/card'
import { Form } from '@/components/ui/form'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { Button } from '@/components/ui/button'
import { useRouter } from 'next/navigation'
import { useIntl } from 'react-intl'
import { MetadataField } from '@/components/form/metadata-field'
import { CountryField, InputField, StateField } from '@/components/form'
import { organization } from '@/schema/organization'
import { OrganizationsFormColorField } from './organizations-form-color-field'
import { OrganizationsFormAvatarField } from './organizations-form-avatar-field'
import {
  useCreateOrganization,
  useUpdateOrganization
} from '@/client/organizations'
import { LoadingButton } from '@/components/ui/loading-button'
import { omit } from 'lodash'
import { OrganizationsFormParentIdField } from './organizations-form-parent-id-field'
import {
  PaperCollapsible,
  PaperCollapsibleBanner,
  PaperCollapsibleContent
} from '@/components/transactions/primitives/paper-collapsible'
import { usePopulateForm } from '@/lib/form'

type OrganizationsViewProps = {
  data?: OrganizationsType
  onSuccess?: () => void
}

const formSchema = z.object({
  id: organization.id,
  parentOrganizationId: organization.parentOrganizationId,
  legalName: organization.legalName,
  doingBusinessAs: organization.doingBusinessAs,
  legalDocument: organization.legalDocument,
  address: z.object(organization.address),
  metadata: organization.metadata,
  accentColor: organization.accentColor,
  avatar: organization.avatar
})

const defaultValues = {
  legalName: '',
  doingBusinessAs: '',
  legalDocument: '',
  address: {
    line1: '',
    line2: '',
    country: '',
    state: '',
    city: '',
    zipCode: ''
  },
  accentColor: '',
  avatar: '',
  metadata: {}
}

const parseInputMetadata = (data?: Partial<OrganizationFormData>) => ({
  ...data,
  accentColor: data?.metadata?.accentColor,
  avatar: data?.metadata?.avatar,
  metadata:
    omit(data?.metadata, ['accentColor', 'avatar']) || defaultValues.metadata
})

const parseInputData = (data?: OrganizationsType) =>
  Object.assign({}, defaultValues, parseInputMetadata(omit(data, ['status'])))

const parseMetadata = (data?: Partial<OrganizationFormData>) => ({
  ...omit(data, ['accentColor', 'avatar']),
  metadata: {
    ...data?.metadata,
    accentColor: data?.accentColor,
    avatar: data?.avatar
  }
})

export const parseCreateData = (data?: OrganizationFormData) =>
  parseMetadata(data)

export const parseUpdateData = (data?: OrganizationFormData) =>
  parseMetadata(omit(data, ['id', 'legalDocument']))

export type OrganizationFormData = z.infer<typeof formSchema>

export const OrganizationsForm = ({
  data,
  onSuccess
}: OrganizationsViewProps) => {
  const intl = useIntl()
  const router = useRouter()
  const isNewOrganization = !data

  const { mutate: createOrganization, isPending: createPending } =
    useCreateOrganization({
      onSuccess
    })
  const { mutate: updateOrganization, isPending: updatePending } =
    useUpdateOrganization({
      organizationId: data?.id!,
      onSuccess
    })

  const form = useForm<OrganizationFormData>({
    resolver: zodResolver(formSchema),
    defaultValues: parseInputData(data!)
  })

  const metadataValue = form.watch('metadata')

  const handleSubmit = (values: OrganizationFormData) => {
    if (isNewOrganization) {
      createOrganization(parseCreateData(values))
    } else {
      updateOrganization(parseUpdateData(values))
    }
  }

  usePopulateForm(form, data)

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(handleSubmit)}>
        <div className="mb-16 flex gap-6">
          <div className="grow space-y-6">
            <Card.Root className="gap-0 space-x-0 space-y-0 p-0 shadow">
              <Card.Header
                title={
                  isNewOrganization
                    ? intl.formatMessage({
                        id: 'organizations.organizationForm.newOrganization.description',
                        defaultMessage:
                          'Fill in the details of the Organization you wish to create.'
                      })
                    : intl.formatMessage({
                        id: 'organizations.organizationForm.editOrganization.description',
                        defaultMessage: 'View and edit the Organization fields.'
                      })
                }
                className="space-x-0 space-y-0 p-6 text-sm font-medium normal-case text-zinc-400"
              />
              <Separator />

              <CardContent className="grid grid-cols-2 gap-5 p-6">
                {!isNewOrganization && (
                  <InputField
                    name="id"
                    label={intl.formatMessage({
                      id: 'entity.organization.id',
                      defaultMessage: 'Organization ID'
                    })}
                    placeholder={intl.formatMessage({
                      id: 'common.typePlaceholder',
                      defaultMessage: 'Type...'
                    })}
                    control={form.control}
                    readOnly
                  />
                )}

                <InputField
                  name="legalName"
                  label={intl.formatMessage({
                    id: 'entity.organization.legalName',
                    defaultMessage: 'Legal Name'
                  })}
                  placeholder={intl.formatMessage({
                    id: 'common.typePlaceholder',
                    defaultMessage: 'Type...'
                  })}
                  control={form.control}
                />

                <InputField
                  name="doingBusinessAs"
                  label={intl.formatMessage({
                    id: 'entity.organization.doingBusinessAs',
                    defaultMessage: 'Trade Name'
                  })}
                  placeholder={intl.formatMessage({
                    id: 'common.typePlaceholder',
                    defaultMessage: 'Type...'
                  })}
                  control={form.control}
                />

                <InputField
                  name="legalDocument"
                  label={intl.formatMessage({
                    id: 'entity.organization.legalDocument',
                    defaultMessage: 'Document'
                  })}
                  placeholder={intl.formatMessage({
                    id: 'common.typePlaceholder',
                    defaultMessage: 'Type...'
                  })}
                  control={form.control}
                  readOnly={!isNewOrganization}
                />
              </CardContent>

              <Separator />

              <CardContent className="grid grid-cols-2 gap-5 p-6">
                <InputField
                  name="address.line1"
                  label={intl.formatMessage({
                    id: 'entity.address',
                    defaultMessage: 'Address'
                  })}
                  placeholder={intl.formatMessage({
                    id: 'common.typePlaceholder',
                    defaultMessage: 'Type...'
                  })}
                  control={form.control}
                />

                <InputField
                  name="address.line2"
                  label={intl.formatMessage({
                    id: 'entity.address.complement',
                    defaultMessage: 'Complement'
                  })}
                  placeholder={intl.formatMessage({
                    id: 'common.typePlaceholder',
                    defaultMessage: 'Type...'
                  })}
                  control={form.control}
                />

                <CountryField
                  name="address.country"
                  label={intl.formatMessage({
                    id: 'entity.address.country',
                    defaultMessage: 'Country'
                  })}
                  placeholder={intl.formatMessage({
                    id: 'common.selectPlaceholder',
                    defaultMessage: 'Select...'
                  })}
                  control={form.control}
                />

                <StateField
                  name="address.state"
                  label={intl.formatMessage({
                    id: 'entity.address.state',
                    defaultMessage: 'State'
                  })}
                  placeholder={intl.formatMessage({
                    id: 'common.selectPlaceholder',
                    defaultMessage: 'Select...'
                  })}
                  control={form.control}
                />

                <InputField
                  name="address.city"
                  label={intl.formatMessage({
                    id: 'entity.address.city',
                    defaultMessage: 'City'
                  })}
                  placeholder={intl.formatMessage({
                    id: 'common.typePlaceholder',
                    defaultMessage: 'Type...'
                  })}
                  control={form.control}
                />

                <InputField
                  name="address.zipCode"
                  label={intl.formatMessage({
                    id: 'entity.address.zipCode',
                    defaultMessage: 'ZIP Code'
                  })}
                  placeholder={intl.formatMessage({
                    id: 'common.typePlaceholder',
                    defaultMessage: 'Type...'
                  })}
                  control={form.control}
                />
              </CardContent>

              <Separator />

              <CardContent className="grid grid-cols-2 gap-5 p-6">
                <OrganizationsFormParentIdField
                  name="parentOrganizationId"
                  label={intl.formatMessage({
                    id: 'entity.organization.parentOrganization',
                    defaultMessage: 'Parent Organization'
                  })}
                  placeholder={intl.formatMessage({
                    id: 'common.selectPlaceholder',
                    defaultMessage: 'Select...'
                  })}
                  description={intl.formatMessage({
                    id: 'organizations.organizationForm.parentOrganizationText',
                    defaultMessage:
                      'Select if your Organization is affiliated with another'
                  })}
                  control={form.control}
                />
              </CardContent>
            </Card.Root>

            <PaperCollapsible className="mb-32">
              <PaperCollapsibleBanner className="flex items-center justify-between">
                <p className="text-lg font-medium normal-case text-zinc-600">
                  {intl.formatMessage({
                    id: 'common.metadata',
                    defaultMessage: 'Metadata'
                  })}
                </p>
                <p className="text-xs italic text-shadcn-400">
                  {intl.formatMessage(
                    {
                      id: 'organizations.organizationForm.metadataRegisterCountText',
                      defaultMessage:
                        '{count} added {count, plural, =0 {records} one {record} other {records}}'
                    },
                    {
                      count: Object.entries(metadataValue || 0).length
                    }
                  )}
                </p>
              </PaperCollapsibleBanner>
              <PaperCollapsibleContent>
                <Separator orientation="horizontal" />
                <div className="p-6">
                  <MetadataField name="metadata" control={form.control} />
                </div>
              </PaperCollapsibleContent>
            </PaperCollapsible>
          </div>

          <div className="grow space-y-6">
            <Card.Root className="p-6 shadow">
              <Card.Header
                className="text-md w-full font-medium normal-case text-zinc-600"
                title={intl.formatMessage({
                  id: 'entity.organization.avatar',
                  defaultMessage: 'Avatar'
                })}
              />

              <CardContent className="p-0">
                <OrganizationsFormAvatarField
                  name="avatar"
                  description={intl.formatMessage({
                    id: 'organizations.organizationForm.avatarInformationText',
                    defaultMessage:
                      'Organization Symbol, which will be applied in the UI. \nFormat: SVG or PNG, 512x512 px.'
                  })}
                  control={form.control}
                />
              </CardContent>
            </Card.Root>

            <Card.Root className="hidden p-6 shadow">
              <Card.Header
                className="text-sm font-medium text-zinc-600"
                title={intl.formatMessage({
                  id: 'entity.organization.accentColor',
                  defaultMessage: 'Accent Color'
                })}
              />

              <CardContent className="flex items-start justify-start gap-2 rounded-lg p-0">
                <OrganizationsFormColorField
                  name="accentColor"
                  description={intl.formatMessage({
                    id: 'organizations.organizationForm.accentColorInformationText',
                    defaultMessage:
                      'Brand color, which will be used specifically in the UI. \nFormat: Hexadecimal/HEX (Ex. #FF0000);'
                  })}
                  control={form.control}
                />
              </CardContent>
            </Card.Root>
          </div>
        </div>

        <div className="relative h-10">
          <CardFooter className="absolute inset-x-0 mb-20 inline-flex items-center justify-end gap-6 self-baseline rounded-none bg-white p-8 shadow">
            <div className="mr-10 flex items-center justify-end gap-6">
              <Button
                variant="secondary"
                type="button"
                onClick={() => router.back()}
              >
                {intl.formatMessage({
                  id: 'common.cancel',
                  defaultMessage: 'Cancel'
                })}
              </Button>
              <LoadingButton
                type="submit"
                loading={createPending || updatePending}
              >
                {isNewOrganization
                  ? intl.formatMessage({
                      id: 'organizations.organizationForm.createOrganization',
                      defaultMessage: 'Create Organization'
                    })
                  : intl.formatMessage({
                      id: 'common.save',
                      defaultMessage: 'Save'
                    })}
              </LoadingButton>
            </div>
          </CardFooter>
        </div>
      </form>
    </Form>
  )
}
