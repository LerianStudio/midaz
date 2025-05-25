declare module 'fs/promises' {
  export function readFile(path: string, encoding: string): Promise<string>;
  export function writeFile(path: string, data: string, encoding?: string): Promise<void>;
  export function readdir(path: string): Promise<string[]>;
  export function mkdir(path: string, options?: { recursive?: boolean }): Promise<string | undefined>;
  export function access(path: string): Promise<void>;
  export function unlink(path: string): Promise<void>;
}

declare module 'path' {
  export function join(...paths: string[]): string;
  export function dirname(path: string): string;
  export function basename(path: string): string;
  export function resolve(...paths: string[]): string;
}