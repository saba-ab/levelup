import React, { createContext, useContext, useState, useEffect, ReactNode, useCallback } from 'react';
import { useToast } from '@/hooks/use-toast';

export type EnvironmentType = 'production' | 'staging' | 'develop' | 'localhost' | 'custom';

export interface Environment {
  id: string;
  name: string;
  url: string;
  type: EnvironmentType;
  isDefault?: boolean;
}

interface EnvironmentContextType {
  environments: Environment[];
  activeEnvironment: Environment;
  isProduction: boolean;
  baseUrl: string;
  setActiveEnvironment: (envId: string) => void;
  addEnvironment: (name: string, url: string) => Environment;
  removeEnvironment: (envId: string) => void;
  getApiUrl: (path: string) => string;
}

// Get environment variables with fallbacks
const getEnvVar = (key: string, fallback: string): string => {
  return import.meta.env[key] || fallback;
};

const defaultEnvironments: Environment[] = [
  {
    id: 'prod',
    name: 'Production',
    url: getEnvVar('VITE_API_URL_PRODUCTION', 'https://api.levelupos.ge'),
    type: 'production',
    isDefault: true
  },
  {
    id: 'staging',
    name: 'Staging',
    url: getEnvVar('VITE_API_URL_STAGING', 'https://staging-api.levelupos.ge'),
    type: 'staging'
  },
  {
    id: 'develop',
    name: 'Development',
    url: getEnvVar('VITE_API_URL_DEVELOPMENT', 'https://dev-api.levelupos.ge'),
    type: 'develop'
  },
  {
    id: 'localhost',
    name: 'Localhost',
    url: getEnvVar('VITE_API_URL_LOCALHOST', 'http://127.0.0.1:8000/api/v1'),
    type: 'localhost'
  },
];

const EnvironmentContext = createContext<EnvironmentContextType | undefined>(undefined);

export function EnvironmentProvider({ children }: { children: ReactNode }) {
  const { toast } = useToast();

  const [environments, setEnvironments] = useState<Environment[]>(() => {
    const stored = localStorage.getItem('levelupos_environments');
    if (stored) {
      try {
        const parsed = JSON.parse(stored);
        // If environment variables have changed, update the URLs in stored environments
        const updated = parsed.map((env: Environment) => {
          const defaultEnv = defaultEnvironments.find(e => e.id === env.id);
          if (defaultEnv && defaultEnv.url !== env.url) {
            // Update URL from environment variable if it exists
            return { ...env, url: defaultEnv.url };
          }
          return env;
        });
        return updated;
      } catch {
        return defaultEnvironments;
      }
    }
    return defaultEnvironments;
  });

  const [activeEnvId, setActiveEnvId] = useState<string>(() => {
    // Check environment variable first (takes precedence)
    const envDefault = getEnvVar('VITE_DEFAULT_ENVIRONMENT', '');

    // If environment variable is set, use it (and clear localStorage to respect it)
    if (envDefault) {
      const stored = localStorage.getItem('levelupos_active_env');
      // Only use localStorage if it matches the env var, otherwise use env var
      if (stored === envDefault) {
        return envDefault;
      }
      // Clear localStorage if env var is different, so env var takes precedence
      if (stored && stored !== envDefault) {
        localStorage.removeItem('levelupos_active_env');
      }
      return envDefault;
    }

    // Fallback to localStorage if no env var is set
    const stored = localStorage.getItem('levelupos_active_env');
    return stored || 'prod';
  });

  const activeEnvironment = environments.find(e => e.id === activeEnvId) || environments[0];
  const isProduction = activeEnvironment?.type === 'production';
  const baseUrl = activeEnvironment?.url || '';

  // Persist environments
  useEffect(() => {
    localStorage.setItem('levelupos_environments', JSON.stringify(environments));
  }, [environments]);

  // Persist active environment
  useEffect(() => {
    localStorage.setItem('levelupos_active_env', activeEnvId);
  }, [activeEnvId]);

  const setActiveEnvironment = useCallback((envId: string) => {
    const env = environments.find(e => e.id === envId);
    if (!env) return;

    if (activeEnvironment?.type === 'production' && env.type !== 'production') {
      toast({
        title: 'Production Mode',
        description: 'Environment switching is disabled in production.',
        variant: 'destructive',
      });
      return;
    }

    setActiveEnvId(envId);
    toast({
      title: 'Environment Changed',
      description: `Now connected to ${env.name} (${env.url})`,
    });
  }, [environments, activeEnvironment, toast]);

  const addEnvironment = useCallback((name: string, url: string): Environment => {
    const newEnv: Environment = {
      id: `custom_${Date.now()}`,
      name: name.trim(),
      url: url.trim(),
      type: 'custom',
    };
    setEnvironments(prev => [...prev, newEnv]);
    toast({ title: 'Environment Added', description: `${newEnv.name} has been added.` });
    return newEnv;
  }, [toast]);

  const removeEnvironment = useCallback((envId: string) => {
    const env = environments.find(e => e.id === envId);
    if (env?.isDefault) {
      toast({ title: 'Cannot Remove', description: 'Default environments cannot be removed.', variant: 'destructive' });
      return;
    }
    setEnvironments(prev => prev.filter(e => e.id !== envId));
    if (activeEnvId === envId) {
      setActiveEnvId('prod');
    }
    toast({ title: 'Environment Removed', description: 'Custom environment has been removed.' });
  }, [environments, activeEnvId, toast]);

  const getApiUrl = useCallback((path: string): string => {
    const cleanPath = path.startsWith('/') ? path : `/${path}`;
    return `${baseUrl}${cleanPath}`;
  }, [baseUrl]);

  return (
    <EnvironmentContext.Provider value={{
      environments,
      activeEnvironment,
      isProduction,
      baseUrl,
      setActiveEnvironment,
      addEnvironment,
      removeEnvironment,
      getApiUrl,
    }}>
      {children}
    </EnvironmentContext.Provider>
  );
}

export function useEnvironment() {
  const context = useContext(EnvironmentContext);
  if (context === undefined) {
    throw new Error('useEnvironment must be used within an EnvironmentProvider');
  }
  return context;
}