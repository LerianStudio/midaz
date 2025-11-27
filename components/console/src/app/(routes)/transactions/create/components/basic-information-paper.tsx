import { useListAssets } from '@/client/assets'
import { InputField, SelectField } from '@/components/form'
import { Paper } from '@/components/ui/paper'
import { SelectItem } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'
import { useOrganization } from '@lerianstudio/console-layout'
import { Control } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { useTransactionRoutesConfig } from '@/hooks/use-transaction-routes-config'

export type BasicInformationPaperProps = {
  className?: string
  control: Control<any>
}

export const BasicInformationPaper = ({
  className,
  control
}: BasicInformationPaperProps) => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()

  // Hook para configuração de routes
  const {
    shouldUseRoutes,
    transactionRoutes,
    isLoading: isLoadingRoutes
  } = useTransactionRoutesConfig()

  const { data: assets } = useListAssets({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id!
  })

  // Preparar opções do select
  const transactionRouteOptions = transactionRoutes?.map((route) => ({
    value: route.id,
    label: route.title
  }))

  return (
    <Paper className={cn('flex flex-col', className)}>
      <div className="grid grid-cols-2 gap-5 p-6">
        <InputField
          name="description"
          label={intl.formatMessage({
            id: 'transactions.field.description',
            defaultMessage: 'Transaction description'
          })}
          description={intl.formatMessage({
            id: 'common.optional',
            defaultMessage: 'Optional'
          })}
          type="text"
          control={control}
          maxHeight={100}
          textArea
        />
        <div className="flex flex-col gap-5">
          <InputField
            name="chartOfAccountsGroupName"
            label={intl.formatMessage({
              id: 'transactions.create.field.chartOfAccountsGroupName',
              defaultMessage: 'Accounting route group'
            })}
            description={intl.formatMessage({
              id: 'common.optional',
              defaultMessage: 'Optional'
            })}
            control={control}
          />
          {/* Campo de Transaction Route */}
        </div>
        {shouldUseRoutes && (
          <div className="flex flex-col gap-5">
            <SelectField
              name="transactionRoute"
              label={intl.formatMessage({
                id: 'common.transactionRoutes',
                defaultMessage: 'Transaction Route'
              })}
              control={control}
              placeholder={intl.formatMessage({
                id: 'transactions.transactionRoute.placeholder',
                defaultMessage: 'Select a transaction route'
              })}
              required={true}
              disabled={isLoadingRoutes || transactionRoutes.length === 0}
            >
              {transactionRoutes.map((route) => (
                <SelectItem key={route.id} value={route.id}>
                  {route.title}
                </SelectItem>
              ))}
            </SelectField>
            {transactionRoutes.length === 0 && !isLoadingRoutes && (
              <p className="text-muted-foreground text-xs">
                {intl.formatMessage({
                  id: 'transactions.transactionRoute.empty',
                  defaultMessage:
                    'No transaction routes available. Please create one first.'
                })}
              </p>
            )}
          </div>
        )}
      </div>
      <Separator orientation="horizontal" />
      <div className="grid grid-cols-4 gap-5 p-6">
        <SelectField
          name="asset"
          label={intl.formatMessage({
            id: 'entity.transaction.asset',
            defaultMessage: 'Asset'
          })}
          control={control}
          required={true}
        >
          {assets?.items?.map((asset) => (
            <SelectItem key={asset.code} value={asset.code}>
              {asset.code}
            </SelectItem>
          ))}
        </SelectField>
        <div className="col-span-2">
          <InputField
            name="value"
            type="number"
            label={intl.formatMessage({
              id: 'common.value',
              defaultMessage: 'Value'
            })}
            control={control}
            required={true}
          />
        </div>
      </div>
    </Paper>
  )
}
