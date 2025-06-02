import { Meta, StoryObj } from '@storybook/nextjs'
import {
  Breadcrumb,
  BreadcrumbEllipsis,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbProps,
  BreadcrumbSeparator
} from '.'

const meta: Meta<BreadcrumbProps> = {
  title: 'Primitives/Breadcrumb',
  component: Breadcrumb,
  argTypes: {
    className: {
      type: 'string'
    }
  }
}

export default meta

export const Primary: StoryObj<BreadcrumbProps> = {
  render: (args) => (
    <Breadcrumb {...args}>
      <BreadcrumbList>
        <BreadcrumbItem>
          <BreadcrumbLink>Home</BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator />
        <BreadcrumbItem>
          <BreadcrumbLink>Components</BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator />
        <BreadcrumbItem>
          <BreadcrumbPage>Breadcrumb</BreadcrumbPage>
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>
  )
}
