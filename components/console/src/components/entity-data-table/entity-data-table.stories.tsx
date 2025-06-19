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

const meta: Meta<React.HTMLAttributes<HTMLDivElement>> = {
  title: 'Components/EntityDataTable',
  component: EntityDataTable.Root,
  argTypes: {}
}

export default meta

export const Primary: StoryObj<React.HTMLAttributes<HTMLDivElement>> = {
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
          Showing 3 items.
        </EntityDataTable.FooterText>
      </EntityDataTable.Footer>
    </EntityDataTable.Root>
  )
}
