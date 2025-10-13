'use client'

import React from 'react'
import { Card } from '@/components/card'
import { Separator } from '@/components/ui/separator'
import { CardContent } from '@/components/ui/card'
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
import { getInitialValues } from '@/lib/form'
import { useFormPermissions } from '@/hooks/use-form-permissions'
import { Enforce } from '@lerianstudio/console-layout'
import { PageFooter, PageFooterSection } from '@/components/page-footer'
import { OrganizationDto } from '@/core/application/dto/organization-dto'

type OrganizationsViewProps = {
  data?: OrganizationDto
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

const initialValues: OrganizationFormData = {
  id: '',
  parentOrganizationId: '',
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
  metadata: data?.metadata || initialValues.metadata
})

const parseInputData = (data?: OrganizationDto) =>
  Object.assign({}, initialValues, parseInputMetadata(omit(data, ['status'])))

export const parseCreateData = (data?: OrganizationFormData) => data

export const parseUpdateData = (data?: OrganizationFormData) =>
  omit(data, ['id', 'legalDocument'])

export type OrganizationFormData = z.infer<typeof formSchema>

export const OrganizationsForm = ({
  data,
  onSuccess
}: OrganizationsViewProps) => {
  const intl = useIntl()
  const router = useRouter()
  const isNewOrganization = !data
  const { isReadOnly } = useFormPermissions('organizations')

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
    values: getInitialValues(initialValues, parseInputData(data)),
    defaultValues: initialValues
  })

  const metadataValue = form.watch('metadata')

  const handleSubmit = (values: OrganizationFormData) => {
    if (isNewOrganization) {
      createOrganization(parseCreateData(values))
    } else {
      updateOrganization(parseUpdateData(values))
    }
  }

  return (
    <Form {...form}>
      <div className="mb-16 flex gap-6" data-testid="organizations-form">
        <div className="grow space-y-6">
          <Card.Root className="gap-0 space-y-0 space-x-0 p-0 shadow-sm">
            <Card.Header
              title={
                isNewOrganization
                  ? intl.formatMessage({
                      id: 'organizations.organizationForm.newOrganization.description',
                      defaultMessage:
                        'Fill in the details of the Organization you wish to create.'
                    })
                  : isReadOnly
                    ? intl.formatMessage({
                        id: 'organizations.organizationForm.viewOrganization.description',
                        defaultMessage:
                          'View the Organization fields in read-only mode.'
                      })
                    : intl.formatMessage({
                        id: 'organizations.organizationForm.editOrganization.description',
                        defaultMessage: 'View and edit the Organization fields.'
                      })
              }
              className="space-y-0 space-x-0 p-6 text-sm font-medium text-zinc-400 normal-case"
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
                  data-testid="organization-id-input"
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
                readOnly={isReadOnly}
                data-testid="organization-legal-name-input"
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
                readOnly={isReadOnly}
                data-testid="organization-doing-business-as-input"
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
                readOnly={!isNewOrganization || isReadOnly}
                data-testid="organization-legal-document-input"
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
                readOnly={isReadOnly}
                data-testid="organization-address-line1-input"
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
                readOnly={isReadOnly}
                data-testid="organization-address-line2-input"
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
                readOnly={isReadOnly}
                data-testid="organization-address-country-select"
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
                readOnly={isReadOnly}
                data-testid="organization-address-state-select"
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
                readOnly={isReadOnly}
                data-testid="organization-address-city-input"
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
                readOnly={isReadOnly}
                data-testid="organization-address-zipcode-input"
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
                readOnly={isReadOnly}
              />
            </CardContent>
          </Card.Root>

          <PaperCollapsible className="mb-32">
            <PaperCollapsibleBanner className="flex items-center justify-between">
              <p className="text-lg font-medium text-zinc-600 normal-case">
                {intl.formatMessage({
                  id: 'common.metadata',
                  defaultMessage: 'Metadata'
                })}
              </p>
              <p className="text-shadcn-400 text-xs italic">
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
                <MetadataField
                  name="metadata"
                  control={form.control}
                  readOnly={isReadOnly}
                />
              </div>
            </PaperCollapsibleContent>
          </PaperCollapsible>
        </div>

        <div className="grow space-y-6">
          <Card.Root className="p-6 shadow-sm">
            <Card.Header
              className="text-md w-full font-medium text-zinc-600 normal-case"
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
                readOnly={isReadOnly}
              />
            </CardContent>
          </Card.Root>

          <Card.Root className="hidden p-6 shadow-sm">
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
                readOnly={isReadOnly}
              />
            </CardContent>
          </Card.Root>
        </div>
      </div>

      <PageFooter open={form.formState.isDirty}>
        <PageFooterSection>
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
        </PageFooterSection>
        <PageFooterSection>
          <Enforce resource="organizations" action="post, patch">
            <LoadingButton
              type="submit"
              onClick={form.handleSubmit(handleSubmit)}
              loading={createPending || updatePending}
              data-testid="organization-form-save-button"
            >
              {intl.formatMessage({
                id: 'common.save',
                defaultMessage: 'Save'
              })}
            </LoadingButton>
          </Enforce>
        </PageFooterSection>
      </PageFooter>
    </Form>
  )
}
