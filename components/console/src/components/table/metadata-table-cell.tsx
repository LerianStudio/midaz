import { TableCell } from '@/components/ui/table'
import { Metadata } from '@/types/metadata-type'
import { TdHTMLAttributes } from 'react'
import { useIntl } from 'react-intl'

export type MetadataTableCellProps = TdHTMLAttributes<HTMLTableCellElement> & {
  metadata?: Metadata
}

export const MetadataTableCell = ({
  metadata,
  ...others
}: MetadataTableCellProps) => {
  const intl = useIntl()

  return (
    <TableCell {...others}>
      {intl.formatMessage(
        {
          id: 'common.table.metadata',
          defaultMessage:
            '{number, plural, =0 {-} one {# record} other {# records}}'
        },
        {
          number: Object.entries(metadata || []).length
        }
      )}
    </TableCell>
  )
}
