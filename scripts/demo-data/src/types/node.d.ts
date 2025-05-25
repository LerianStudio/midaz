
declare global {
  const process: NodeJS.Process;
  const require: NodeRequire;
  
  namespace NodeJS {
    interface Process {
      env: ProcessEnv;
      exit(code?: number): never;
      cwd(): string;
      argv: string[];
      memoryUsage(): {
        rss: number;
        heapTotal: number;
        heapUsed: number;
        external: number;
        arrayBuffers: number;
      };
    }
    
    interface ProcessEnv {
      NODE_ENV?: string;
      [key: string]: string | undefined;
    }
    
    interface Timer {
      ref(): this;
      unref(): this;
      hasRef(): boolean;
      refresh(): this;
    }
    
    type Timeout = Timer;
  }
  
  interface NodeRequire {
    (id: string): any;
  }
}

export {};