'use client'

import React from 'react'
import Image from 'next/image'
import LoadingImage from '@/images/loading-wallpaper.webp'
import midazLoading from '@/animations/midaz-loading.json'
import { Lottie } from '@/lib/lottie'

type LoadingScreenProps = {
  onComplete?: () => void
}

const LoadingScreen = ({ onComplete }: LoadingScreenProps) => {
  return (
    <div className="relative flex h-screen w-screen items-center justify-center">
      <Image src={LoadingImage} alt="Loading image" fill sizes="100vw, 100vh" />
      <div className="absolute inset-0 bg-black bg-opacity-50"></div>
      <div className="relative z-10 h-36 w-36">
        <Lottie
          animationData={midazLoading}
          loop={false}
          onComplete={onComplete}
        />
      </div>
    </div>
  )
}

export default LoadingScreen
