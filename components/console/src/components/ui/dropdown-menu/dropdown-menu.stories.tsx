import { Meta, StoryObj } from '@storybook/nextjs'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuItemIcon,
  DropdownMenuLabel,
  DropdownMenuPortal,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger
} from '.'
import {
  Cloud,
  CreditCard,
  Github,
  Keyboard,
  LifeBuoy,
  LogOut,
  Mail,
  MessageSquare,
  Plus,
  PlusCircle,
  Settings,
  User,
  UserPlus,
  Users
} from 'lucide-react'
import { Button } from '../button'
import { DropdownMenuProps } from '@radix-ui/react-dropdown-menu'

const meta: Meta<DropdownMenuProps> = {
  title: 'Primitives/DropdownMenu',
  component: DropdownMenu,
  argTypes: {}
}
export default meta

type Story = StoryObj<DropdownMenuProps>

export const Primary: Story = {
  render: (args) => (
    <DropdownMenu {...args}>
      <DropdownMenuTrigger asChild>
        <Button variant="outline">Open</Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-56">
        <DropdownMenuLabel>My Account</DropdownMenuLabel>
        <DropdownMenuSeparator />

        <DropdownMenuGroup>
          <DropdownMenuItem>
            <DropdownMenuItemIcon>
              <User />
            </DropdownMenuItemIcon>
            <span>Profile</span>
            <DropdownMenuShortcut>⇧⌘P</DropdownMenuShortcut>
          </DropdownMenuItem>

          <DropdownMenuItem>
            <DropdownMenuItemIcon>
              <CreditCard />
            </DropdownMenuItemIcon>
            <span>Billing</span>
            <DropdownMenuShortcut>⌘B</DropdownMenuShortcut>
          </DropdownMenuItem>

          <DropdownMenuItem>
            <DropdownMenuItemIcon>
              <Settings />
            </DropdownMenuItemIcon>
            <span>Settings</span>
            <DropdownMenuShortcut>⌘S</DropdownMenuShortcut>
          </DropdownMenuItem>

          <DropdownMenuItem>
            <DropdownMenuItemIcon>
              <Keyboard />
            </DropdownMenuItemIcon>
            <span>Keyboard shortcuts</span>
            <DropdownMenuShortcut>⌘K</DropdownMenuShortcut>
          </DropdownMenuItem>
        </DropdownMenuGroup>

        <DropdownMenuSeparator />

        <DropdownMenuGroup>
          <DropdownMenuItem>
            <DropdownMenuItemIcon>
              <Users />
            </DropdownMenuItemIcon>
            <span>Team</span>
          </DropdownMenuItem>

          <DropdownMenuSub>
            <DropdownMenuSubTrigger>
              <DropdownMenuItemIcon>
                <UserPlus />
              </DropdownMenuItemIcon>
              <span>Invite users</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuPortal>
              <DropdownMenuSubContent>
                <DropdownMenuItem>
                  <DropdownMenuItemIcon>
                    <Mail />
                  </DropdownMenuItemIcon>
                  <span>Email</span>
                </DropdownMenuItem>

                <DropdownMenuItem>
                  <DropdownMenuItemIcon>
                    <MessageSquare />
                  </DropdownMenuItemIcon>
                  <span>Message</span>
                </DropdownMenuItem>

                <DropdownMenuSeparator />

                <DropdownMenuItem>
                  <DropdownMenuItemIcon>
                    <PlusCircle />
                  </DropdownMenuItemIcon>
                  <span>More...</span>
                </DropdownMenuItem>
              </DropdownMenuSubContent>
            </DropdownMenuPortal>
          </DropdownMenuSub>

          <DropdownMenuItem>
            <DropdownMenuItemIcon>
              <Plus />
            </DropdownMenuItemIcon>
            <span>New Team</span>
            <DropdownMenuShortcut>⌘+T</DropdownMenuShortcut>
          </DropdownMenuItem>
        </DropdownMenuGroup>

        <DropdownMenuSeparator />

        <DropdownMenuItem>
          <DropdownMenuItemIcon>
            <Github />
          </DropdownMenuItemIcon>
          <span>GitHub</span>
        </DropdownMenuItem>

        <DropdownMenuItem>
          <DropdownMenuItemIcon>
            <LifeBuoy />
          </DropdownMenuItemIcon>
          <span>Support</span>
        </DropdownMenuItem>

        <DropdownMenuItem disabled>
          <DropdownMenuItemIcon>
            <Cloud />
          </DropdownMenuItemIcon>
          <span>API</span>
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        <DropdownMenuItem>
          <DropdownMenuItemIcon>
            <LogOut />
          </DropdownMenuItemIcon>
          <span>Log out</span>
          <DropdownMenuShortcut>⇧⌘Q</DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
