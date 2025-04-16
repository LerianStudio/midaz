import React, { useState } from 'react'
import { Input } from '@/components/ui/input'
import Image from 'next/image'
import { Camera } from 'lucide-react'

export function AvatarInputFile() {
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [previewUrl, setPreviewUrl] = useState<string | null>(null)

  const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files ? event.target.files[0] : null
    if (file) {
      setSelectedFile(file)
      setPreviewUrl(URL.createObjectURL(file))
    }
  }

  return (
    <div className="flex w-full max-w-sm items-center justify-center">
      <div
        className="relative cursor-pointer overflow-hidden rounded-full bg-gray-200 dark:bg-black"
        style={{ width: '100px', height: '100px' }}
        onClick={() => document?.getElementById('picture')?.click()}
      >
        {previewUrl ? (
          <Image
            src={previewUrl}
            alt="Avatar"
            className="h-full w-full object-cover"
            width={100}
            height={100}
          />
        ) : (
          <div className="flex h-full items-center justify-center">
            <Camera />
          </div>
        )}
        <Input
          id="picture"
          type="file"
          className="hidden"
          onChange={handleFileChange}
          accept="image/*"
        />
      </div>
    </div>
  )
}
