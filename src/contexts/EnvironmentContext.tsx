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

const defaultEnvironments: Environment[] = [
  { id: 'prod', name: 'Production', url: 'https://api.levelupos.com', type: 'production', isDefault: true },
  { id: 'staging', name: 'Staging', url: 'https://staging-api.levelupos.com', type: 'staging' },
  { id: 'develop', name: 'Development', url: 'https://dev-api.levelupos.com', type: 'develop' },
  { id: 'localhost', name: 'Localhost', url: 'http://localhost:3001', type: 'localhost' },
];

const EnvironmentContext = createContext<EnvironmentContextType | undefined>(undefined);

export function EnvironmentProvider({ children }: { children: ReactNode }) {
  const { toast } = useToast();
  
  const [environments, setEnvironments] = useState<Environment[]>(() => {
    const stored = localStorage.getItem('levelupos_environments');
    return stored ? JSON.parse(stored) : defaultEnvironments;
  });

  const [activeEnvId, setActiveEnvId] = useState<string>(() => {
    return localStorage.getItem('levelupos_active_env') || 'prod';
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