/* eslint-disable @next/next/no-img-element */

import { Meta, StoryObj } from '@storybook/nextjs'
import {
  ArrowLeftRight,
  BarChartHorizontal,
  Box,
  Briefcase,
  Coins,
  DatabaseZap,
  DollarSign,
  Home,
  UsersRound
} from 'lucide-react'
import {
  SidebarRoot,
  SidebarHeader,
  SidebarContent,
  SidebarGroup,
  SidebarItem,
  SidebarGroupTitle,
  SidebarExpandButton,
  SidebarProvider,
  useSidebar
} from '.'

const meta: Meta = {
  title: 'Components/Sidebar',
  component: SidebarRoot,
  argTypes: {}
}

export default meta

function Template() {
  const { isCollapsed } = useSidebar()

  return (
    <SidebarRoot>
      <SidebarHeader>
        <img className="w-10" alt="" src="/svg/lerian-logo.svg" />
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarItem title="Home" icon={<Home />} href="/" />
          <SidebarItem title="Ledgers" icon={<DatabaseZap />} href="/ledgers" />
          <SidebarItem title="Team" icon={<UsersRound />} href="/team" />
        </SidebarGroup>

        <SidebarGroup>
          <SidebarGroupTitle collapsed={isCollapsed}>
            AccountHolders
          </SidebarGroupTitle>
          <SidebarItem title="Segments" icon={<Box />} href="/segments" />
          <SidebarItem title="Accounts" icon={<Coins />} href="/accounts" />
          <SidebarItem
            title="Portfolios"
            icon={<Briefcase />}
            href="/portfolios"
          />
        </SidebarGroup>

        <SidebarGroup>
          <SidebarGroupTitle collapsed={isCollapsed}>
            Transactions
          </SidebarGroupTitle>
          <SidebarItem title="Types" icon={<DollarSign />} href="/types" />
          <SidebarItem
            title="Resume"
            icon={<ArrowLeftRight />}
            href="/resume"
          />
        </SidebarGroup>

        <SidebarGroup>
          <SidebarGroupTitle collapsed={isCollapsed}>Reports</SidebarGroupTitle>
          <SidebarItem
            title="Run Report"
            icon={<BarChartHorizontal />}
            href="/reports"
          />
        </SidebarGroup>
      </SidebarContent>

      <SidebarExpandButton />
    </SidebarRoot>
  )
}

export const Primary: StoryObj = {
  render: () => (
    <SidebarProvider>
      <div className="flex h-[640px]">
        <Template />
      </div>
    </SidebarProvider>
  )
}
