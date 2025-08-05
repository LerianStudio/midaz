import { useIntl } from 'react-intl'

export type CardButtonProps = {
  icon: React.ReactNode
  warning?: React.ReactNode
  title: string
  subtitle: string
  onClick?: () => void
}

export const CardButton = ({
  icon,
  warning,
  title,
  subtitle,
  onClick
}: CardButtonProps) => {
  const intl = useIntl()

  return (
    <div
      className="group hover:border-accent hover:bg-accent flex w-80 cursor-pointer flex-col gap-8 rounded-[8px] border border-zinc-200 bg-white p-6 transition-colors"
      onClick={onClick}
    >
      <div className="flex flex-row justify-between text-zinc-400 transition-colors group-hover:text-zinc-800">
        {icon}
        {warning}
      </div>
      <h3 className="text-2xl font-extrabold text-zinc-700 transition-colors group-hover:text-zinc-800">
        {title}
      </h3>
      <p className="text-sm font-normal text-zinc-500 transition-colors group-hover:text-zinc-800 group-hover:opacity-80">
        {subtitle}
      </p>
      <p className="text-sm font-medium text-zinc-600 transition-colors group-hover:text-zinc-800">
        {intl.formatMessage({
          id: 'common.select',
          defaultMessage: 'Select'
        })}
      </p>
    </div>
  )
}
