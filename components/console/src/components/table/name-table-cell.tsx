import { Search } from 'lucide-react'
import { TableCell, TableCellWrapper, TableCellAction } from '../ui/table'

export type NameTableCellProps = React.ComponentProps<typeof TableCell> & {
  name?: string | React.ReactNode
}

export const NameTableCell = ({ name, ...props }: NameTableCellProps) => {
  return (
    <TableCell {...props}>
      <TableCellWrapper>
        <p className="cursor-pointer">{name}</p>
        <TableCellAction>
          <Search className="size-3.5" />
        </TableCellAction>
      </TableCellWrapper>
    </TableCell>
  )
}
