import { Meta, StoryObj } from '@storybook/react'
import {
  AccountBalanceCard,
  AccountBalanceCardContent,
  AccountBalanceCardDeleteButton,
  AccountBalanceCardEmpty,
  AccountBalanceCardHeader,
  AccountBalanceCardIcon,
  AccountBalanceCardInfo,
  AccountBalanceCardLoading,
  AccountBalanceCardTitle,
  AccountBalanceCardTrigger,
  AccountBalanceCardUpdateButton
} from './account-balance-card'
import { Separator } from '@/components/ui/separator'
import React from 'react'

const meta: Meta = {
  title: 'Components/Transactions/AccountBalanceCard',
  component: AccountBalanceCard,
  parameters: {
    backgrounds: {
      default: 'Light'
    }
  },
  argTypes: {}
}

export default meta

export const Primary: StoryObj = {
  render: (args) => {
    const [time, setTime] = React.useState(Date.now())

    return (
      <AccountBalanceCard className="w-80" {...args}>
        <AccountBalanceCardHeader>
          <AccountBalanceCardTitle>krasinski</AccountBalanceCardTitle>
          <AccountBalanceCardDeleteButton />
        </AccountBalanceCardHeader>
        <AccountBalanceCardContent>
          <AccountBalanceCardInfo assetCode="USD" value={999999999.0} />
          <AccountBalanceCardInfo assetCode="BRL" value={10000.0} />

          <Separator className="mb-2 mt-3" />
          <AccountBalanceCardUpdateButton
            timestamp={time}
            onRefresh={() => setTime(Date.now())}
          />
        </AccountBalanceCardContent>
        <AccountBalanceCardTrigger />
      </AccountBalanceCard>
    )
  }
}

export const Icon: StoryObj = {
  render: (args) => {
    return (
      <AccountBalanceCard className="w-80" {...args}>
        <AccountBalanceCardHeader>
          <AccountBalanceCardIcon />
          <AccountBalanceCardTitle>krasinski</AccountBalanceCardTitle>
          <AccountBalanceCardDeleteButton />
        </AccountBalanceCardHeader>
        <AccountBalanceCardContent>
          <AccountBalanceCardInfo assetCode="USD" value={999999999.0} />

          <Separator className="mb-2 mt-3" />
          <AccountBalanceCardUpdateButton
            timestamp={Date.now()}
            onRefresh={() => {}}
          />
        </AccountBalanceCardContent>
        <AccountBalanceCardTrigger />
      </AccountBalanceCard>
    )
  }
}

export const NoExpand: StoryObj = {
  render: (args) => {
    return (
      <AccountBalanceCard className="w-80" {...args}>
        <AccountBalanceCardHeader>
          <AccountBalanceCardTitle>krasinski</AccountBalanceCardTitle>
          <AccountBalanceCardDeleteButton />
        </AccountBalanceCardHeader>
      </AccountBalanceCard>
    )
  }
}

export const Loading: StoryObj = {
  render: (args) => {
    return (
      <AccountBalanceCard className="w-80" open {...args}>
        <AccountBalanceCardHeader>
          <AccountBalanceCardTitle>krasinski</AccountBalanceCardTitle>
          <AccountBalanceCardDeleteButton />
        </AccountBalanceCardHeader>
        <AccountBalanceCardContent>
          <AccountBalanceCardLoading />
          <Separator className="mb-2 mt-3" />
          <AccountBalanceCardUpdateButton
            loading={true}
            timestamp={Date.now()}
            onRefresh={() => {}}
          />
        </AccountBalanceCardContent>
        <AccountBalanceCardTrigger />
      </AccountBalanceCard>
    )
  }
}

export const Empty: StoryObj = {
  render: (args) => {
    return (
      <AccountBalanceCard className="w-80" open {...args}>
        <AccountBalanceCardHeader>
          <AccountBalanceCardTitle>krasinski</AccountBalanceCardTitle>
          <AccountBalanceCardDeleteButton />
        </AccountBalanceCardHeader>
        <AccountBalanceCardContent>
          <AccountBalanceCardEmpty />
          <Separator className="mb-2 mt-3" />
          <AccountBalanceCardUpdateButton
            timestamp={Date.now()}
            onRefresh={() => {}}
          />
        </AccountBalanceCardContent>
        <AccountBalanceCardTrigger />
      </AccountBalanceCard>
    )
  }
}

export const Simple: StoryObj = {
  render: (args) => {
    return (
      <AccountBalanceCard className="w-80" open {...args}>
        <AccountBalanceCardHeader>
          <AccountBalanceCardTitle>krasinski</AccountBalanceCardTitle>
        </AccountBalanceCardHeader>
      </AccountBalanceCard>
    )
  }
}
