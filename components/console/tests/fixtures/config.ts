import dotenv from 'dotenv'

dotenv.config({ path: './.env.playwright' })

export const {
  MIDAZ_CONSOLE_HOST,
  MIDAZ_CONSOLE_PORT,
  MIDAZ_USERNAME,
  MIDAZ_PASSWORD,
  MIDAZ_BASE_PATH,
  MIDAZ_TRANSACTION_BASE_PATH,
  ORGANIZATION_ID,
  LEDGER_ID,
  DB_HOST,
  DB_PORT,
  DB_USER,
  DB_PASSWORD,
  DB_NAME
} = process.env

export const MIDAZ_CONSOLE_URL = `http://${MIDAZ_CONSOLE_HOST}:${MIDAZ_CONSOLE_PORT}`
