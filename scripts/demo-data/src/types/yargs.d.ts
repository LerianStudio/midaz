declare module 'yargs' {
  const yargs: any;
  export default yargs;
}

declare module 'yargs/helpers' {
  export function hideBin(argv: string[]): string[];
}