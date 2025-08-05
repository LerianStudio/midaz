#!/bin/sh

# Generates the public/runtime-env.js file with the container's environment variables
echo "window.RUNTIME_ENV = $(node -p 'JSON.stringify({
  NEXT_PUBLIC_MIDAZ_CONSOLE_AVATAR_ALLOWED_FORMAT: process.env.NEXT_PUBLIC_MIDAZ_CONSOLE_AVATAR_ALLOWED_FORMAT,
  NEXT_PUBLIC_MIDAZ_APPLICATION_OPTIONS: process.env.NEXT_PUBLIC_MIDAZ_APPLICATION_OPTIONS,
  NEXT_PUBLIC_MIDAZ_CONSOLE_BASE_URL: process.env.NEXT_PUBLIC_MIDAZ_CONSOLE_BASE_URL,
  NEXT_PUBLIC_MIDAZ_AUTH_ENABLED: process.env.NEXT_PUBLIC_MIDAZ_AUTH_ENABLED,
  NEXT_PUBLIC_MIDAZ_VERSION: process.env.NEXT_PUBLIC_MIDAZ_VERSION
})');" > ./public/runtime-env.js

# Executes the original command (Next.js start or any passed command)
exec "$@"
