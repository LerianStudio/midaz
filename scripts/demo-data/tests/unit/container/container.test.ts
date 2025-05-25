/**
 * Tests for Dependency Injection Container
 */

import { Container, ServiceTokens } from '../../../src/container/container';

describe('Container', () => {
  let container: Container;

  beforeEach(() => {
    container = new Container();
  });

  describe('register and resolve', () => {
    it('should register and resolve a service', () => {
      const service = { name: 'TestService' };
      container.register('test', () => service);

      const resolved = container.resolve('test');
      expect(resolved).toBe(service);
    });

    it('should register and resolve with symbol token', () => {
      const token = Symbol('test');
      const service = { name: 'TestService' };
      container.register(token, () => service);

      const resolved = container.resolve(token);
      expect(resolved).toBe(service);
    });

    it('should throw error for unregistered service', () => {
      expect(() => container.resolve('unknown')).toThrow("Service 'unknown' not registered");
    });
  });

  describe('singleton behavior', () => {
    it('should return same instance for singleton services', () => {
      let callCount = 0;
      container.registerSingleton('singleton', () => {
        callCount++;
        return { id: callCount };
      });

      const instance1 = container.resolve('singleton');
      const instance2 = container.resolve('singleton');

      expect(instance1).toBe(instance2);
      expect(callCount).toBe(1);
    });

    it('should return new instance for transient services', () => {
      let callCount = 0;
      container.registerTransient('transient', () => {
        callCount++;
        return { id: callCount };
      });

      const instance1 = container.resolve('transient');
      const instance2 = container.resolve('transient');

      expect(instance1).not.toBe(instance2);
      expect(instance1.id).toBe(1);
      expect(instance2.id).toBe(2);
      expect(callCount).toBe(2);
    });
  });

  describe('registerValue', () => {
    it('should register a value directly', () => {
      const config = { apiUrl: 'http://localhost' };
      container.registerValue('config', config);

      const resolved = container.resolve('config');
      expect(resolved).toBe(config);
    });
  });

  describe('circular dependency detection', () => {
    it('should detect direct circular dependency', () => {
      container.register('a', () => container.resolve('a'));

      expect(() => container.resolve('a')).toThrow('Circular dependency detected for service \'a\'');
    });

    it('should detect indirect circular dependency', () => {
      container.register('a', () => container.resolve('b'));
      container.register('b', () => container.resolve('a'));

      expect(() => container.resolve('a')).toThrow('Circular dependency detected');
    });
  });

  describe('async resolution', () => {
    it('should resolve async factories', async () => {
      const service = { name: 'AsyncService' };
      container.register('async', async () => {
        await new Promise(resolve => setTimeout(resolve, 10));
        return service;
      });

      const resolved = await container.resolveAsync('async');
      expect(resolved).toBe(service);
    });

    it('should cache async singleton results', async () => {
      let callCount = 0;
      container.registerSingleton('asyncSingleton', async () => {
        callCount++;
        await new Promise(resolve => setTimeout(resolve, 10));
        return { id: callCount };
      });

      const [instance1, instance2] = await Promise.all([
        container.resolveAsync('asyncSingleton'),
        container.resolveAsync('asyncSingleton'),
      ]);

      expect(instance1).toBe(instance2);
      expect(callCount).toBe(1);
    });
  });

  describe('utility methods', () => {
    it('should check if service is registered', () => {
      container.register('test', () => ({}));

      expect(container.has('test')).toBe(true);
      expect(container.has('unknown')).toBe(false);
    });

    it('should get all registered tokens', () => {
      const token1 = 'service1';
      const token2 = Symbol('service2');
      
      container.register(token1, () => ({}));
      container.register(token2, () => ({}));

      const tokens = container.getTokens();
      expect(tokens).toContain(token1);
      expect(tokens).toContain(token2);
      expect(tokens).toHaveLength(2);
    });

    it('should clear all services', () => {
      container.register('service1', () => ({}));
      container.register('service2', () => ({}));

      container.clear();

      expect(container.has('service1')).toBe(false);
      expect(container.has('service2')).toBe(false);
      expect(container.getTokens()).toHaveLength(0);
    });
  });

  describe('child containers', () => {
    it('should create child container with parent services', () => {
      const parentService = { name: 'ParentService' };
      container.register('parent', () => parentService);

      const child = container.createChild();
      const resolved = child.resolve('parent');

      expect(resolved).toBe(parentService);
    });

    it('should allow child to override parent services', () => {
      const parentService = { name: 'ParentService' };
      const childService = { name: 'ChildService' };
      
      container.register('service', () => parentService);
      const child = container.createChild();
      child.register('service', () => childService);

      expect(container.resolve('service')).toBe(parentService);
      expect(child.resolve('service')).toBe(childService);
    });
  });

  describe('ServiceTokens', () => {
    it('should have unique symbols for all services', () => {
      const tokens = Object.values(ServiceTokens);
      const uniqueTokens = new Set(tokens);
      
      expect(uniqueTokens.size).toBe(tokens.length);
    });
  });
});