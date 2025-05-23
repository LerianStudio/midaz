'use client'

import { default as BaseImage, ImageProps } from 'next/image'
import dynamic from 'next/dynamic'

const Image = (props: ImageProps) => <BaseImage {...props} />

export default dynamic(() => Promise.resolve(Image), {
  ssr: false
})
