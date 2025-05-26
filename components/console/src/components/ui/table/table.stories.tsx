import { Meta, StoryObj } from '@storybook/react'
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow
} from '.'

const meta: Meta<React.HTMLAttributes<HTMLTableElement>> = {
  title: 'Primitives/Table',
  component: Table,
  argTypes: {}
}
export default meta

function Template(props: React.HTMLAttributes<HTMLTableElement>) {
  return (
    <Table {...props}>
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
  )
}

type Story = StoryObj<React.HTMLAttributes<HTMLTableElement>>

export const Primary: Story = {
  render: (args) => <Template {...args} />
}

export const WithContainer: Story = {
  render: (args) => (
    <TableContainer>
      <Template {...args} />
    </TableContainer>
  )
}
