import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib/utils'

const buttonVariants = cva(
  'relative flex inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors disabled:pointer-events-none disabled:opacity-50',
  {
    variants: {
      variant: {
        plain: '',
        white: 'bg-white text-black font-semibold',
        activeLink: 'bg-shadcn-100 text-black font-medium',
        hoverLink:
          'hover:bg-accent text-black hover:text-accent-foreground font-normal',
        default:
          'bg-primary text-primary-foreground hover:bg-primary/90 shadow-sm disabled:bg-shadcn-200 disabled:text-shadcn-600',
        destructive:
          'bg-destructive text-destructive-foreground hover:bg-destructive/90',
        outline:
          'border border-shadcn-300 bg-background hover:bg-accent hover:text-accent-foreground shadow-sm',
        secondary:
          'border border-shadcn-300 bg-background hover:bg-primary/5 text-secondary-foreground shadow-sm',
        ghost: 'hover:bg-shadcn-300',
        link: 'text-shadcn-600 underline-offset-4 underline text-sm font-normal justify-start font-medium'
      },
      size: {
        default: 'h-10 px-4 py-2',
        sm: 'h-8 rounded-md px-3 py-2',
        lg: 'h-12 rounded-md px-8',
        icon: 'h-10 w-10',
        link: 'p-0 w-auto h-auto',
        xl: 'h-14 p-4'
      }
    },
    defaultVariants: {
      variant: 'default',
      size: 'default'
    }
  }
)

const iconVariants = cva('', {
  variants: {
    position: {
      start: 'mr-2',
      end: 'ml-2',
      'far-end': 'absolute right-2'
    },
    size: {
      default: 'h-10',
      sm: 'h-8',
      lg: 'h-12',
      icon: 'h-10',
      link: 'h-6',
      xl: 'h-14'
    }
  },
  defaultVariants: {
    position: 'start'
  }
})

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean
  icon?: React.ReactNode
  iconPlacement?: 'start' | 'end' | 'far-end'
  fullWidth?: boolean
  readOnly?: boolean
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      className,
      variant,
      size,
      asChild = false,
      icon,
      iconPlacement = 'start',
      fullWidth,
      readOnly,
      ...props
    },
    ref
  ) => {
    const Comp = asChild ? Slot : 'button'

    return (
      <Comp
        className={cn(
          buttonVariants({ variant, size, className }),
          {
            'w-full': fullWidth
          },
          readOnly && [
            'data-[read-only]:cursor-default',
            'data-[read-only]:select-text',
            'data-[read-only]:bg-zinc-100',
            'data-[read-only]:opacity-50',
            'data-[read-only]:hover:bg-zinc-100',
            'data-[read-only]:hover:text-current',
            'data-[read-only]:focus:outline-none',
            'data-[read-only]:focus:ring-0',
            'data-[read-only]:focus:ring-offset-0',
            'data-[read-only]:caret-transparent'
          ]
        )}
        data-read-only={readOnly ? '' : undefined}
        ref={ref}
        {...props}
      >
        {icon && iconPlacement === 'start' && (
          <span className={cn(iconVariants({ position: iconPlacement }))}>
            {icon}
          </span>
        )}
        {props.children}
        {icon && iconPlacement !== 'start' && (
          <span className={cn(iconVariants({ position: iconPlacement }))}>
            {icon}
          </span>
        )}
      </Comp>
    )
  }
)
Button.displayName = 'Button'

export { Button, buttonVariants }
