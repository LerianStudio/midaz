'use client'

import React from 'react'
import Image from 'next/image'
import LoadingImage from '@/images/bg-wallpaper.webp'
import LerianLoading from '@/animations/lerian-loading.json'
import { Lottie } from '@/lib/lottie'

type LoadingScreenProps = {
  onComplete?: () => void
}

const LoadingScreen = ({ onComplete }: LoadingScreenProps) => {
  return (
    <div className="relative flex h-screen w-screen items-center justify-center">
      <Image src={LoadingImage} alt="Loading image" fill sizes="100vw, 100vh" />

      <div className="relative z-10 h-60 w-60">
        <Lottie
          animationData={LerianLoading}
          loop={false}
          onComplete={onComplete}
        />
      </div>
    </div>
  )
}

export default LoadingScreen
