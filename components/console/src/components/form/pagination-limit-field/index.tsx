import { Control } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { SelectField } from '../select-field'
import { SelectItem } from '@/components/ui/select'

export type PaginationLimitFieldProps = {
  options?: number[]
  control: Control<any>
}

export const PaginationLimitField = ({
  options = [10, 50, 100],
  control
}: PaginationLimitFieldProps) => {
  const intl = useIntl()

  return (
    <div className="flex items-center gap-4">
      <p className="text-sm font-medium whitespace-nowrap text-gray-600">
        {intl.formatMessage({
          id: 'common.itemsPerPage',
          defaultMessage: 'Items per page'
        })}
      </p>
      <SelectField
        name="limit"
        control={control}
        data-testid="pagination-limit"
      >
        {options.map((pageSize: number) => (
          <SelectItem key={pageSize} value={String(pageSize)}>
            {pageSize}
          </SelectItem>
        ))}
      </SelectField>
    </div>
  )
}
