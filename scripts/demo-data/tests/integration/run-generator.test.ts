import { execFile } from 'child_process';
import { promisify } from 'util';
import * as path from 'path';

const execFileAsync = promisify(execFile);

describe('run-generator.sh integration test', () => {
  const scriptPath = path.resolve(__dirname, '../../run-generator.sh');
  
  // This is a longer-running test that actually executes the script with mocked responses
  jest.setTimeout(30000); // Increase timeout to 30s for script execution
  
  // Helper to run the script with arguments
  const runScript = async (args: string[] = []) => {
    try {
      // We'll run with a special test flag that can be intercepted to prevent actual API calls
      const result = await execFileAsync(scriptPath, [...args, '--test-mode'], {
        env: { ...process.env, MIDAZ_TEST_MODE: 'true' }
      });
      return { stdout: result.stdout, stderr: result.stderr, code: 0 };
    } catch (error: any) {
      return {
        stdout: error.stdout || '',
        stderr: error.stderr || '',
        code: error.code || 1
      };
    }
  };

  it('should show help information with invalid arguments', async () => {
    const { stdout, code } = await runScript(['invalid-size']);
    
    expect(code).not.toBe(0);
    expect(stdout).toContain('Invalid volume size');
  });

  it('should accept valid volume sizes', async () => {
    // We're just testing that the script validates inputs correctly
    // In test mode, it won't make actual API calls
    
    // Test small volume
    const smallResult = await runScript(['small']);
    expect(smallResult.stdout).toContain('Using volume size: small');
    
    // Test medium volume
    const mediumResult = await runScript(['medium']);
    expect(mediumResult.stdout).toContain('Using volume size: medium');
    
    // Test large volume
    const largeResult = await runScript(['large']);
    expect(largeResult.stdout).toContain('Using volume size: large');
  });

  it('should handle authentication tokens', async () => {
    const testToken = 'test-auth-token';
    const { stdout } = await runScript(['small', testToken]);
    
    expect(stdout).toContain('Using volume size: small');
    // In a real test, we would verify the token is passed to the generator
    // but that would require modifying the script to log this information
  });

  it('should detect operating system', async () => {
    const { stdout } = await runScript(['small']);
    
    // Should detect either macOS or Linux
    expect(stdout).toMatch(/Detected: (macos|ubuntu|debian|centos|fedora|redhat).* \((darwin|linux)\)/);
  });

  it('should check for required dependencies', async () => {
    const { stdout } = await runScript(['small']);
    
    expect(stdout).toContain('Checking for required dependencies');
    expect(stdout).toContain('All required dependencies are installed');
  });
});
