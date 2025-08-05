import { Meta, StoryObj } from '@storybook/nextjs'
import React from 'react'
import { EntityDataTable } from '.'
import {
  TableHeader,
  TableRow,
  TableHead,
  TableBody,
  TableCell,
  Table
} from '../ui/table'
import { IdTableCell } from '../table/id-table-cell'
import { NameTableCell } from '../table/name-table-cell'

const meta: Meta = {
  title: 'Components/EntityDataTable',
  component: EntityDataTable.Root,
  argTypes: {}
}

export default meta

export const Primary: StoryObj = {
  render: (args) => (
    <EntityDataTable.Root {...args}>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Type</TableHead>
            <TableHead>Code</TableHead>
            <TableHead>Metadata</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell>American Dolar</TableCell>
            <TableCell>Coin</TableCell>
            <TableCell>USD</TableCell>
            <TableCell>2 registers</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Bitcoin</TableCell>
            <TableCell>Cripto</TableCell>
            <TableCell>BTC</TableCell>
            <TableCell>-</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>Tether</TableCell>
            <TableCell>Cripto</TableCell>
            <TableCell>USDT</TableCell>
            <TableCell>4 registers</TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <EntityDataTable.Footer>
        <EntityDataTable.FooterText>
          Showing <b>3</b> items.
        </EntityDataTable.FooterText>
      </EntityDataTable.Footer>
    </EntityDataTable.Root>
  )
}

export const WithRowActions: StoryObj = {
  render: (args) => (
    <EntityDataTable.Root {...args}>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>ID</TableHead>
            <TableHead>Code</TableHead>
            <TableHead>Metadata</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow active>
            <NameTableCell name="American Dolar" />
            <IdTableCell id="344ee35b-4bd8-40af-a0ae-0ef9cb84e878" />
            <TableCell>USD</TableCell>
            <TableCell>2 registers</TableCell>
          </TableRow>
          <TableRow>
            <NameTableCell name="Bitcoin" />
            <IdTableCell id="344ee35b-4bd8-40af-a0ae-0ef9cb84e878" />
            <TableCell>BTC</TableCell>
            <TableCell>-</TableCell>
          </TableRow>
          <TableRow>
            <NameTableCell name="Tether" />
            <IdTableCell id="344ee35b-4bd8-40af-a0ae-0ef9cb84e878" />
            <TableCell>USDT</TableCell>
            <TableCell>4 registers</TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <EntityDataTable.Footer>
        <EntityDataTable.FooterText>
          Showing <b>3</b> items.
        </EntityDataTable.FooterText>
      </EntityDataTable.Footer>
    </EntityDataTable.Root>
  )
}
