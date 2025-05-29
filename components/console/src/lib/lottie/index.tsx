import dynamic from 'next/dynamic'

export const Lottie = dynamic(() => import('lottie-react'), { ssr: false })
