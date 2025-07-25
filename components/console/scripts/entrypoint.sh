#!/bin/sh

# Gera o arquivo public/runtime-env.js com as envs do container
echo "window.RUNTIME_ENV = $(node -p 'JSON.stringify({
  NEXT_PUBLIC_MIDAZ_CONSOLE_AVATAR_ALLOWED_FORMAT: process.env.NEXT_PUBLIC_MIDAZ_CONSOLE_AVATAR_ALLOWED_FORMAT,
  NEXT_PUBLIC_MIDAZ_APPLICATION_OPTIONS: process.env.NEXT_PUBLIC_MIDAZ_APPLICATION_OPTIONS,
  NEXT_PUBLIC_MIDAZ_CONSOLE_BASE_URL: process.env.NEXT_PUBLIC_MIDAZ_CONSOLE_BASE_URL,
  NEXT_PUBLIC_MIDAZ_AUTH_ENABLED: process.env.NEXT_PUBLIC_MIDAZ_AUTH_ENABLED,
  NEXT_PUBLIC_MIDAZ_VERSION: process.env.NEXT_PUBLIC_MIDAZ_VERSION
})');" > ./public/runtime-env.js

# Executa o comando original (start do Next.js, ou qualquer comando passado)
exec "$@"
