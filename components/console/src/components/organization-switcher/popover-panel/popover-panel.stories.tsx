/* eslint-disable @next/next/no-img-element */

import { Meta, StoryObj } from '@storybook/react'
import {
  PopoverPanel,
  PopoverPanelActions,
  PopoverPanelContent,
  PopoverPanelFooter,
  PopoverPanelLink,
  PopoverPanelProps,
  PopoverPanelTitle
} from '.'
import { StatusDisplay } from '../status'
import Link from 'next/link'
import { ArrowRight, Settings } from 'lucide-react'

const meta: Meta<PopoverPanelProps> = {
  title: 'Components/OrganizationSwitcherPopover',
  component: PopoverPanel,
  argTypes: {}
}

export default meta

export const Primary: StoryObj<PopoverPanelProps> = {
  render: (args) => (
    <div className="z-50 flex w-[530px] gap-4 rounded-md border bg-popover p-4 text-popover-foreground shadow-md outline-none">
      <PopoverPanel {...args}>
        <PopoverPanelTitle>
          Midaz
          <StatusDisplay status="active" />
        </PopoverPanelTitle>
        <PopoverPanelContent>
          <img
            src="/svg/brand-midaz.svg"
            alt=""
            className="rounded-full"
            height={24}
          />
        </PopoverPanelContent>
        <PopoverPanelFooter>
          <Link href="">Edit</Link>
        </PopoverPanelFooter>
      </PopoverPanel>

      <PopoverPanelActions>
        <PopoverPanelLink href="" icon={<ArrowRight />} onClick={() => {}}>
          <img src="/svg/brand-midaz.svg" alt="" className="w-6 rounded-full" />
          Midaz
        </PopoverPanelLink>

        <PopoverPanelLink
          href="/settings?tab=organizations"
          icon={<Settings />}
          onClick={() => {}}
        >
          Organization
        </PopoverPanelLink>
      </PopoverPanelActions>
    </div>
  )
}

export const Dense: StoryObj<PopoverPanelProps> = {
  render: (args) => (
    <div className="z-50 flex w-[530px] gap-4 rounded-md border bg-popover p-4 text-popover-foreground shadow-md outline-none">
      <PopoverPanel {...args}>
        <PopoverPanelTitle>
          Midaz
          <StatusDisplay status="active" />
        </PopoverPanelTitle>
        <PopoverPanelContent>
          <img
            src="/svg/brand-midaz.svg"
            alt=""
            className="rounded-full"
            height={24}
          />
        </PopoverPanelContent>
        <PopoverPanelFooter>
          <Link href="">Edit</Link>
        </PopoverPanelFooter>
      </PopoverPanel>

      <PopoverPanelActions>
        <PopoverPanelLink
          href=""
          dense
          icon={<ArrowRight />}
          onClick={() => {}}
        >
          <img src="/svg/brand-midaz.svg" alt="" className="w-6 rounded-full" />
          Midaz
        </PopoverPanelLink>
        <PopoverPanelLink
          href=""
          dense
          icon={<ArrowRight />}
          onClick={() => {}}
        >
          <img src="/svg/brand-midaz.svg" alt="" className="w-6 rounded-full" />
          Midaz
        </PopoverPanelLink>
        <PopoverPanelLink
          href=""
          dense
          icon={<ArrowRight />}
          onClick={() => {}}
        >
          <img src="/svg/brand-midaz.svg" alt="" className="w-6 rounded-full" />
          Midaz
        </PopoverPanelLink>

        <PopoverPanelLink
          href="/settings?tab=organizations"
          icon={<Settings />}
          onClick={() => {}}
          dense
        >
          Organization
        </PopoverPanelLink>
      </PopoverPanelActions>
    </div>
  )
}
