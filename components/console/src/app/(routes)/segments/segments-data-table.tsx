import React from 'react'
import { useIntl } from 'react-intl'
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import { EmptyResource } from '@/components/empty-resource'
import { Button } from '@/components/ui/button'
import { MoreVertical } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { isNil } from 'lodash'
import { EntityDataTable } from '@/components/entity-data-table'
import { Pagination, PaginationProps } from '@/components/pagination'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { IdTableCell } from '@/components/table/id-table-cell'
import { MetadataTableCell } from '@/components/table/metadata-table-cell'
import { SegmentDto } from '@/core/application/dto/segment-dto'

type SegmentsTableProps = {
  segments: PaginationDto<SegmentDto> | undefined
  table: {
    getRowModel: () => {
      rows: { id: string; original: SegmentDto }[]
    }
  }
  handleDialogOpen: (id: string, name: string) => void
  handleCreate: () => void
  handleEdit: (asset: SegmentDto) => void
  refetch: () => void
  total: number
  pagination: PaginationProps
}

type SegmentRowProps = {
  segment: { id: string; original: SegmentDto }
  handleDialogOpen: (id: string, name: string) => void
  handleEdit: (segment: SegmentDto) => void
}

const SegmentRow: React.FC<SegmentRowProps> = ({
  segment,
  handleDialogOpen,
  handleEdit
}) => {
  const intl = useIntl()

  return (
    <TableRow key={segment.id}>
      <TableCell>{segment.original.name}</TableCell>
      <IdTableCell id={segment.original.id} />
      <MetadataTableCell metadata={segment.original.metadata!} />
      <TableCell className="w-0" align="center">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button className="h-[34px] w-[34px] p-2" variant="secondary">
              <MoreVertical size={16} />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem onClick={() => handleEdit(segment.original)}>
              {intl.formatMessage({
                id: `common.details`,
                defaultMessage: 'Details'
              })}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={() =>
                handleDialogOpen(
                  segment.original.id || '',
                  segment.original.name || ''
                )
              }
            >
              {intl.formatMessage({
                id: `common.delete`,
                defaultMessage: 'Delete'
              })}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </TableCell>
    </TableRow>
  )
}

export const SegmentsDataTable: React.FC<SegmentsTableProps> = (props) => {
  const intl = useIntl()

  const {
    segments,
    table,
    handleDialogOpen,
    handleCreate,
    handleEdit,
    pagination,
    total
  } = props

  return (
    <>
      <EntityDataTable.Root>
        {isNil(segments?.items) || segments.items.length === 0 ? (
          <EmptyResource
            message={intl.formatMessage({
              id: 'ledgers.segments.emptyResource',
              defaultMessage: "You haven't created any Segments yet"
            })}
          >
            <Button onClick={handleCreate}>
              {intl.formatMessage({
                id: 'ledgers.segments.sheet.title',
                defaultMessage: 'New Segment'
              })}
            </Button>
          </EmptyResource>
        ) : (
          <TableContainer>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.name',
                      defaultMessage: 'Name'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.id',
                      defaultMessage: 'ID'
                    })}
                  </TableHead>
                  <TableHead>
                    {intl.formatMessage({
                      id: 'common.metadata',
                      defaultMessage: 'Metadata'
                    })}
                  </TableHead>
                  <TableHead className="w-0">
                    {intl.formatMessage({
                      id: 'common.actions',
                      defaultMessage: 'Actions'
                    })}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {table.getRowModel().rows.map((segment) => (
                  <SegmentRow
                    key={segment.id}
                    segment={segment}
                    handleDialogOpen={handleDialogOpen}
                    handleEdit={handleEdit}
                  />
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}

        <EntityDataTable.Footer>
          <EntityDataTable.FooterText>
            {intl.formatMessage(
              {
                id: 'ledgers.segments.showing',
                defaultMessage:
                  '{number, plural, =0 {No segments found} one {Showing {count} segment} other {Showing {count} segments}}.'
              },
              {
                number: segments?.items?.length || 0,
                count: (
                  <span className="font-bold">{segments?.items?.length}</span>
                )
              }
            )}
          </EntityDataTable.FooterText>
          <Pagination total={total} {...pagination} />
        </EntityDataTable.Footer>
      </EntityDataTable.Root>
    </>
  )
}
