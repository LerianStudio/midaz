const CPF_BLOCKLIST = new Set([
  '00000000000',
  '11111111111',
  '22222222222',
  '33333333333',
  '44444444444',
  '55555555555',
  '66666666666',
  '77777777777',
  '88888888888',
  '99999999999'
]);

const CNPJ_BLOCKLIST = new Set([
  '00000000000000',
  '11111111111111',
  '22222222222222',
  '33333333333333',
  '44444444444444',
  '55555555555555',
  '66666666666666',
  '77777777777777',
  '88888888888888',
  '99999999999999'
]);

const configuredCpfs = (__ENV.K6_TEST_CPFS || '')
  .split(',')
  .map((value) => value.trim())
  .filter((value) => /^\d{11}$/.test(value));

const configuredCnpjs = (__ENV.K6_TEST_CNPJS || '')
  .split(',')
  .map((value) => value.trim())
  .filter((value) => /^\d{14}$/.test(value));

let cpfIndex = 0;
let cnpjIndex = 0;

function randomDigit(maxExclusive = 10) {
  return Math.floor(Math.random() * maxExclusive);
}

export function generateCPF() {
  if (configuredCpfs.length > 0) {
    const value = configuredCpfs[cpfIndex % configuredCpfs.length];
    cpfIndex++;
    return value;
  }

  while (true) {
    const digits = Array.from({ length: 9 }, () => randomDigit(10));

    let d1 = digits.reduce((sum, num, i) => sum + num * (10 - i), 0);
    d1 = 11 - (d1 % 11);
    d1 = d1 >= 10 ? 0 : d1;

    let d2 = digits.reduce((sum, num, i) => sum + num * (11 - i), 0) + d1 * 2;
    d2 = 11 - (d2 % 11);
    d2 = d2 >= 10 ? 0 : d2;

    const cpf = `${digits.join('')}${d1}${d2}`;

    if (!CPF_BLOCKLIST.has(cpf)) {
      return cpf;
    }
  }
}

export function generateCNPJ() {
  if (configuredCnpjs.length > 0) {
    const value = configuredCnpjs[cnpjIndex % configuredCnpjs.length];
    cnpjIndex++;
    return value;
  }

  while (true) {
    const digits = Array.from({ length: 8 }, () => randomDigit(10));
    digits.push(0, 0, 0, 1);

    const weights1 = [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2];
    let d1 = digits.reduce((sum, num, i) => sum + num * weights1[i], 0) % 11;
    d1 = d1 < 2 ? 0 : 11 - d1;

    const weights2 = [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2];
    const digitsWithD1 = [...digits, d1];
    let d2 = digitsWithD1.reduce((sum, num, i) => sum + num * weights2[i], 0) % 11;
    d2 = d2 < 2 ? 0 : 11 - d2;

    const cnpj = `${digits.join('')}${d1}${d2}`;

    if (!CNPJ_BLOCKLIST.has(cnpj)) {
      return cnpj;
    }
  }
}

function randomCents(min, max) {
  const minCents = Math.round(Number(min) * 100);
  const maxCents = Math.round(Number(max) * 100);
  const cents = Math.floor(Math.random() * (maxCents - minCents + 1)) + minCents;
  return cents;
}

export function generateAmountString(min = 1, max = 1000) {
  const cents = randomCents(min, max);
  return (cents / 100).toFixed(2);
}

export function generateAmountNumber(min = 1, max = 1000) {
  return Number(generateAmountString(min, max));
}
