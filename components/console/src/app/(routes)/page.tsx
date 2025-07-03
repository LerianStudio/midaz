'use client'

import Image from 'next/image'
import grafismo from 'public/svg/grafismo.svg'
import banner from 'public/svg/banner.svg'
import { useIntl } from 'react-intl'
import { PageContent, PageRoot, PageView } from '@/components/page'
import { Sidebar } from '@/components/sidebar'
import { Header } from '@/components/header'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Link2 } from 'lucide-react'
import { useRouter } from 'next/navigation'
import { MetricSection } from './metric-section'
import { useOrganization } from '@/providers/organization-provider'
import { Skeleton } from '@/components/ui/skeleton'

// Next Steps Card Component
const NextStepsCard = ({
  title,
  description,
  buttonText,
  onButtonClick
}: {
  title: string
  description: string
  buttonText: string
  onButtonClick: () => void
}) => (
  <Card className="flex flex-1 flex-col gap-6 p-6">
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-2">
        <h3 className="text-sm leading-5 font-medium text-zinc-900">{title}</h3>
      </div>
      <div className="flex flex-col">
        <p className="text-sm leading-[1.4] font-medium text-zinc-500">
          {description}
        </p>
      </div>
    </div>
    <div className="flex justify-center">
      <Button
        onClick={onButtonClick}
        className="h-[37px] w-[280px] text-sm font-medium"
      >
        {buttonText}
      </Button>
    </div>
  </Card>
)

// Dev Resource Link Component
const DevResourceLink = ({ title, href }: { title: string; href: string }) => (
  <div className="flex h-[18px] items-center gap-1.5">
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="text-xs leading-5 font-medium text-zinc-400 transition-colors hover:text-zinc-600"
    >
      {title}
    </a>
  </div>
)

const Page = () => {
  const intl = useIntl()
  const router = useRouter()
  const { currentLedger } = useOrganization()

  return (
    <PageRoot>
      <Sidebar />
      <PageView>
        <Header />
        <PageContent className="p-0">
          <div className="flex flex-col">
            {/* Header Section */}
            <div className="bg-accent flex h-[262px] flex-col items-center gap-8 overflow-hidden">
              <div className="flex w-full gap-24 pr-24 pl-44">
                <div className="flex flex-col gap-4 pt-24">
                  <h1 className="text-4xl leading-[1.21] font-bold text-zinc-900">
                    {currentLedger.name ?? <Skeleton className="h-11 w-64" />}
                  </h1>
                  <p className="text-sm leading-6 font-medium -tracking-[1.1%] text-zinc-800/80">
                    {intl.formatMessage({
                      id: 'homePage.header.description',
                      defaultMessage:
                        "This is the home page of your current ledger. Use the side menu to navigate the entities. Don't know where to start? Check out our suggestions in Next Steps."
                    })}
                  </p>
                </div>
                <Image
                  src={grafismo}
                  alt="Decorative Vector"
                  width={320.23}
                  height={306}
                />
              </div>
            </div>

            {/* Body Section */}
            <div className="flex flex-1 flex-col items-center gap-2.5 bg-zinc-100 py-16">
              <div className="flex w-full flex-col gap-6 px-44">
                {/* Next Steps Section */}
                <div className="flex flex-col gap-3">
                  <div className="flex flex-col gap-4">
                    <h2 className="text-sm leading-5 font-semibold text-zinc-600">
                      {intl.formatMessage({
                        id: 'homePage.nextSteps.title',
                        defaultMessage: 'Next Steps'
                      })}
                    </h2>
                  </div>

                  <div className="flex gap-6">
                    <NextStepsCard
                      title={intl.formatMessage({
                        id: 'common.assets',
                        defaultMessage: 'Assets'
                      })}
                      description={intl.formatMessage({
                        id: 'homePage.nextSteps.assets.description',
                        defaultMessage:
                          'View and manage assets, the financial instruments or currencies that accounts hold.'
                      })}
                      buttonText={intl.formatMessage({
                        id: 'homePage.nextSteps.assets.button',
                        defaultMessage: 'Manage Assets'
                      })}
                      onButtonClick={() => router.push('/assets')}
                    />
                    <NextStepsCard
                      title={intl.formatMessage({
                        id: 'common.accounts',
                        defaultMessage: 'Accounts'
                      })}
                      description={intl.formatMessage({
                        id: 'homePage.nextSteps.accounts.description',
                        defaultMessage:
                          'View and manage accounts, the core financial unit that tracks all debits, credits, and balances.'
                      })}
                      buttonText={intl.formatMessage({
                        id: 'homePage.nextSteps.accounts.button',
                        defaultMessage: 'Manage Accounts'
                      })}
                      onButtonClick={() => router.push('/accounts')}
                    />
                    <NextStepsCard
                      title={intl.formatMessage({
                        id: 'common.transactions',
                        defaultMessage: 'Transactions'
                      })}
                      description={intl.formatMessage({
                        id: 'homePage.nextSteps.transactions.description',
                        defaultMessage:
                          'View and create transactions, the lifeblood that encapsulates every financial movement within your ledger.'
                      })}
                      buttonText={intl.formatMessage({
                        id: 'homePage.nextSteps.transactions.button',
                        defaultMessage: 'View Transactions'
                      })}
                      onButtonClick={() => router.push('/transactions')}
                    />
                  </div>
                </div>

                {/* My Operation Section */}
                <MetricSection />
              </div>
            </div>

            {/* Footer Section */}
            <div className="flex justify-between gap-2.5 bg-white py-8">
              <div className="mx-auto w-full px-44">
                <div className="flex items-center justify-between gap-6">
                  {/* Dev Resources */}
                  <div className="flex flex-col gap-4 py-4">
                    <div className="flex items-center gap-4">
                      <h3 className="text-sm leading-5 font-medium text-zinc-600 uppercase">
                        {intl.formatMessage({
                          id: 'homePage.footer.devResources.title',
                          defaultMessage: 'Dev Resources'
                        })}
                      </h3>
                      <Link2
                        className="h-6 w-6 text-zinc-400"
                        strokeWidth={1.5}
                      />
                    </div>

                    <div className="flex flex-col justify-center gap-2">
                      <DevResourceLink
                        title={intl.formatMessage({
                          id: 'homePage.footer.devResources.midazDocs',
                          defaultMessage: 'Midaz Docs'
                        })}
                        href="https://docs.lerian.studio/"
                      />
                      <DevResourceLink
                        title={intl.formatMessage({
                          id: 'homePage.footer.devResources.lerianDiscord',
                          defaultMessage: 'Lerian Discord'
                        })}
                        href="https://discord.com/invite/DnhqKwkGv3"
                      />
                      <DevResourceLink
                        title={intl.formatMessage({
                          id: 'homePage.footer.devResources.github',
                          defaultMessage: 'Github'
                        })}
                        href="https://github.com/LerianStudio/midaz"
                      />
                    </div>
                  </div>

                  {/* Banner */}
                  <div className="relative">
                    <Image
                      src={banner}
                      alt="Lerian Studio Banner"
                      width={502}
                      height={208}
                    />
                    <Button
                      className="absolute right-11 bottom-9 w-[191px]"
                      onClick={() =>
                        window.open('https://dev.to/lerian', '_blank')
                      }
                    >
                      {intl.formatMessage({
                        id: 'homePage.footer.banner.button',
                        defaultMessage: 'Check out our Blog'
                      })}
                    </Button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </PageContent>
      </PageView>
    </PageRoot>
  )
}

export default Page
