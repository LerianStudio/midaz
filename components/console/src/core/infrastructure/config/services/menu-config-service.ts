import {
  MidazMenuGroupDto,
  MidazMenuItemDto
} from '@/core/application/dto/midaz-menu-dto'
import menuConfigJson from '@/midaz-menus.json'
import { inject, injectable } from 'inversify'
import type { IntlShape } from 'react-intl'
import { TYPES } from '../../container-registry/lib/lib-module'
import { MenuConfigSchema } from '@/schema/midaz-menu'
import { LoggerAggregator } from '@lerianstudio/lib-logs'
import { BadRequestApiException } from '@/lib/http'

type MidazMenuGroupServiceType = {
  id: string
  title: string | null
  titleKey: string | null
  showSeparatorAfter: boolean
  items: MidazMenuServiceType[]
}

type MidazMenuServiceType = {
  name: string
  title: string
  titleKey: string
  route: string
  icon: string
  hasLedgerDependencies: boolean
  order: number
}

export abstract class MenuConfig {
  abstract getMenuConfig(): MidazMenuGroupDto[]
}

@injectable()
export class MenuConfigService implements MenuConfig {
  constructor(
    @inject(TYPES.Intl) private readonly intl: IntlShape,
    @inject(LoggerAggregator) private readonly logger: LoggerAggregator
  ) {}

  getMenuConfig(): MidazMenuGroupDto[] {
    const menus = this.loadMenuConfig()

    return this.getMenus(menus)
  }

  private loadMenuConfig(): MidazMenuGroupServiceType[] {
    try {
      const menus = MenuConfigSchema.parse(menuConfigJson)
      return menus.groups.map((menu) => {
        return {
          id: menu.id,
          title: menu.title,
          titleKey: menu.titleKey,
          showSeparatorAfter: menu.showSeparatorAfter,
          items: menu.items
        }
      })
    } catch (error) {
      this.logger.error(
        '[ERROR] - MenuConfigService - Error parsing menu config',
        {
          error: error
        }
      )
      throw new BadRequestApiException(
        this.intl.formatMessage({
          id: 'errors.midaz.errorParsingMenuConfig',
          defaultMessage: 'Invalid menu configuration'
        })
      )
    }
  }

  private getMenus(menus: MidazMenuGroupServiceType[]): MidazMenuGroupDto[] {
    return menus.map((menu) => {
      const items = this.loadMenuItems(menu)
      const group: MidazMenuGroupDto = {
        id: menu.id,
        showSeparatorAfter: menu.showSeparatorAfter,
        items: items
      }

      if (menu.title) {
        group.title = menu.title
      }

      return group
    })
  }

  private loadMenuItems(menu: MidazMenuGroupServiceType): MidazMenuItemDto[] {
    return menu.items.map((item) => {
      return {
        name: item.name,
        title: this.intl.formatMessage({ id: item.titleKey }),
        host:
          process.env.NEXT_PUBLIC_MIDAZ_CONSOLE_BASE_URL ||
          'http://localhost:8081',
        route: item.route,
        icon: item.icon,
        hasLedgerDependencies: item.hasLedgerDependencies,
        order: item.order
      }
    })
  }
}
