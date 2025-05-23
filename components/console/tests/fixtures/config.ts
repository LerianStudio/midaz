import dotenv from 'dotenv'

dotenv.config({ path: './.env.playwright' })

export const {
  MIDAZ_CONSOLE_HOST,
  MIDAZ_CONSOLE_PORT,
  MIDAZ_USERNAME,
  MIDAZ_PASSWORD
} = process.env

export const MIDAZ_CONSOLE_URL = `http://${MIDAZ_CONSOLE_HOST}:${MIDAZ_CONSOLE_PORT}`
