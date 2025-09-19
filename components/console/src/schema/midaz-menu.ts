import z from 'zod'

const MenuItemSchema = z.object({
  name: z.string(),
  title: z.string(),
  titleKey: z.string(),
  route: z.string(),
  icon: z.string(),
  hasLedgerDependencies: z.boolean(),
  order: z.number()
})

const MenuGroupSchema = z.object({
  id: z.string(),
  title: z.string().nullable(),
  titleKey: z.string().nullable(),
  showSeparatorAfter: z.boolean(),
  items: z.array(MenuItemSchema)
})

export const MenuConfigSchema = z.object({
  groups: z.array(MenuGroupSchema)
})
