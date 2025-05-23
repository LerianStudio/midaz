import type { Config } from 'tailwindcss'

const config = {
  darkMode: ['class'],
  content: [
    './src/components/**/*.{ts,tsx}',
    './src/app/**/*.{ts,tsx}',
    './src/stories/**/*.{js,ts,jsx,tsx,mdx}',
    './src/hooks/**/*.{ts,tsx}'
  ],
  prefix: '',
  theme: {
    container: {
      center: true,
      padding: '2rem',
      screens: {
        '2xl': '1400px'
      }
    },
    extend: {
      boxShadow: {
        sidebar: '5px 0px 15px -3px rgba(0, 0, 0, 0.05)',
        dataTable:
          '0px 1px 2px 0px rgba(0, 0, 0, 0.10), 0px 10px 20px 0px rgba(0, 0, 0, 0.05)',
        sheetBottom:
          '0px -1px 2px 0px rgba(0, 0, 0, 0.10), 0px -10px 20px 0px rgba(0, 0, 0, 0.05)',
        drawer:
          '0px 4px 8px 0px rgba(0, 0, 0, 0.20), 0px -1px 16px 0px rgba(0, 0, 0, 0.10), 0px 0px 32px 0px rgba(0, 0, 0, 0.05);',
        entityBox:
          '0px 10px 20px rgba(0, 0, 0, 0.05), 0px 1px 2px rgba(0, 0, 0, 0.10)'
      },
      colors: {
        sunglow: {
          '50': '#fefbe8',
          '100': '#fff8c2',
          '200': '#ffed89',
          '300': '#ffdc45',
          '400': '#fdcb28',
          '500': '#edac05',
          '600': '#cc8402',
          '700': '#a35d05',
          '800': '#86490d',
          '900': '#723c11',
          '950': '#431e05'
        },
        deYork: {
          '50': '#f2fbf5',
          '100': '#e0f8e8',
          '200': '#c2f0d2',
          '300': '#74db9a',
          '400': '#5bcd86',
          '500': '#35b264',
          '600': '#26934f',
          '700': '#217441',
          '800': '#1f5c36',
          '900': '#1b4c2f',
          '950': '#0a2917'
        },
        vividTangerine: {
          '50': '#fef5f2',
          '100': '#fee9e2',
          '200': '#fed7ca',
          '300': '#fdbaa4',
          '400': '#faa589',
          '500': '#f06e43',
          '600': '#dd5325',
          '700': '#ba421b',
          '800': '#9a3a1a',
          '900': '#80351c',
          '950': '#45190a'
        },
        codGray: {
          '50': '#f6f5f5',
          '100': '#e9e4e4',
          '200': '#d5ccce',
          '300': '#b7a9ab',
          '400': '#927e81',
          '500': '#776366',
          '600': '#655557',
          '700': '#55494a',
          '800': '#4a4040',
          '900': '#403939',
          '950': '#070606'
        },
        shadcn: {
          '100': '#f4f4f5',
          '200': '#e4e4e7',
          '300': '#d4d4d8',
          '400': '#a1a1aa',
          '500': '#71717a',
          '600': '#27272a',
          '700': '#18181b',
          '800': '#09090b'
        },
        border: 'hsl(var(--border))',
        input: 'hsl(var(--input))',
        ring: 'hsl(var(--ring))',
        background: 'hsl(var(--background))',
        foreground: 'hsl(var(--foreground))',
        primary: {
          DEFAULT: 'hsl(var(--primary))',
          foreground: 'hsl(var(--primary-foreground))'
        },
        secondary: {
          DEFAULT: 'hsl(var(--secondary))',
          foreground: 'hsl(var(--secondary-foreground))'
        },
        destructive: {
          DEFAULT: 'hsl(var(--destructive))',
          foreground: 'hsl(var(--destructive-foreground))'
        },
        muted: {
          DEFAULT: 'hsl(var(--muted))',
          foreground: 'hsl(var(--muted-foreground))'
        },
        accent: {
          DEFAULT: 'hsl(var(--accent))',
          foreground: 'hsl(var(--accent-foreground))'
        },
        popover: {
          DEFAULT: 'hsl(var(--popover))',
          foreground: 'hsl(var(--popover-foreground))'
        },
        card: {
          DEFAULT: 'hsl(var(--card))',
          foreground: 'hsl(var(--card-foreground))'
        }
      },
      borderRadius: {
        lg: 'var(--radius)',
        md: 'calc(var(--radius) - 2px)',
        sm: 'calc(var(--radius) - 4px)'
      },
      keyframes: {
        'accordion-down': {
          from: { height: '0' },
          to: { height: 'var(--radix-collapsible-content-height)' }
        },
        'accordion-up': {
          from: { height: 'var(--radix-collapsible-content-height)' },
          to: { height: '0' }
        },
        fill: {
          '0%': { transform: 'scaleY(0)' },
          '100%': { transform: 'scaleY(1)' }
        }
      },
      animation: {
        'accordion-down': 'accordion-down 0.2s ease-out',
        'accordion-up': 'accordion-up 0.2s ease-out',
        fill: 'fill 2s ease-out forwards'
      }
    }
  },
  plugins: [require('tailwindcss-animate')]
} satisfies Config

export default config
