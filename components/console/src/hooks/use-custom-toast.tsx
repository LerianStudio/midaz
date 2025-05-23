import { toast } from 'react-hot-toast'
import { Check, X, AlertTriangle, Info, Ban } from 'lucide-react'
import { cn } from '@/lib/utils'

const customToast = (
  message: string,
  icon: JSX.Element,
  bgColor: string,
  dataTestId?: string
) => {
  toast.custom(
    (t) => (
      <div
        data-testid={dataTestId}
        className={cn(
          'pointer-events-auto flex w-full max-w-[330px] rounded-lg bg-white px-4 py-5 shadow-2xl transition-opacity duration-100 ease-in-out',
          t.visible ? 'opacity-100' : 'opacity-0'
        )}
      >
        <div className="flex flex-1">
          <div className="flex items-center gap-[10px]">
            <div
              className={cn(
                'flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg',
                bgColor
              )}
            >
              {icon}
            </div>
            <div className="w-full min-w-[234px] text-wrap">
              <p className="text-sm font-medium text-shadcn-500">{message}</p>
            </div>
          </div>
          <button
            onClick={() => toast.dismiss(t.id)}
            className="flex"
            data-testid="dismiss-toast"
          >
            <X className="text-[#9CA3AF]" size={20} />
          </button>
        </div>
      </div>
    ),
    { duration: 3000 }
  )
}

const useCustomToast = () => {
  const showSuccess = (message: string) => {
    customToast(
      message,
      <Check size={16} className="text-[#009F6F]" />,
      'bg-[#D1FAE5]',
      'success-toast'
    )
  }

  const showError = (message: string) => {
    customToast(
      message,
      <Ban size={20} className="text-[#EF4444]" />,
      'bg-[#FEE2E2]',
      'error-toast'
    )
  }

  const showInfo = (message: string) => {
    customToast(
      message,
      <Info size={16} className="text-[#2563EB]" />,
      'bg-white',
      'info-toast'
    )
  }

  const showWarning = (message: string) => {
    customToast(
      message,
      <AlertTriangle size={16} className="text-[#FBBF24]" />,
      'bg-yellow-100',
      'warning-toast'
    )
  }

  return { showSuccess, showError, showInfo, showWarning }
}

export default useCustomToast
