import React, { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Server, ChevronUp, Check, Plus, Trash2, Globe, Code, TestTube, Laptop } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useEnvironment, EnvironmentType } from '@/contexts/EnvironmentContext';

const envTypeConfig: Record<EnvironmentType, { icon: React.ReactNode; color: string }> = {
  production: { icon: <Globe className="h-3.5 w-3.5" />, color: 'bg-green-500/20 text-green-400 border-green-500/30' },
  staging: { icon: <TestTube className="h-3.5 w-3.5" />, color: 'bg-amber-500/20 text-amber-400 border-amber-500/30' },
  develop: { icon: <Code className="h-3.5 w-3.5" />, color: 'bg-blue-500/20 text-blue-400 border-blue-500/30' },
  localhost: { icon: <Laptop className="h-3.5 w-3.5" />, color: 'bg-purple-500/20 text-purple-400 border-purple-500/30' },
  custom: { icon: <Server className="h-3.5 w-3.5" />, color: 'bg-cyan-500/20 text-cyan-400 border-cyan-500/30' },
};

export default function EnvironmentSwitcher() {
  const { 
    environments, 
    activeEnvironment, 
    isProduction, 
    setActiveEnvironment, 
    addEnvironment, 
    removeEnvironment 
  } = useEnvironment();
  
  const [isOpen, setIsOpen] = useState(false);
  const [showAddForm, setShowAddForm] = useState(false);
  const [newEnvName, setNewEnvName] = useState('');
  const [newEnvUrl, setNewEnvUrl] = useState('');

  const handleSelectEnvironment = (envId: string) => {
    setActiveEnvironment(envId);
    setIsOpen(false);
  };

  const handleAddEnvironment = () => {
    if (!newEnvName.trim() || !newEnvUrl.trim()) return;
    addEnvironment(newEnvName, newEnvUrl);
    setNewEnvName('');
    setNewEnvUrl('');
    setShowAddForm(false);
  };

  // Don't render in actual production build
  if (import.meta.env.PROD && import.meta.env.MODE === 'production') {
    return null;
  }

  return (
    <div className="fixed bottom-4 right-4 z-50">
      <Popover open={isOpen} onOpenChange={setIsOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            className={cn(
              "gap-2 shadow-lg border-2 bg-background/95 backdrop-blur-sm hover:bg-background",
              envTypeConfig[activeEnvironment.type].color
            )}
          >
            {envTypeConfig[activeEnvironment.type].icon}
            <span className="hidden sm:inline">{activeEnvironment.name}</span>
            <ChevronUp className={cn("h-3 w-3 transition-transform", isOpen && "rotate-180")} />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-80 p-0 bg-background border shadow-xl" sideOffset={8}>
          <div className="p-3 border-b">
            <div className="flex items-center justify-between">
              <h4 className="font-semibold text-sm">Backend Environment</h4>
              {isProduction && (
                <Badge variant="outline" className="text-[10px] bg-destructive/10 text-destructive border-destructive/30">
                  Locked
                </Badge>
              )}
            </div>
            <p className="text-xs text-muted-foreground mt-1">
              {isProduction 
                ? 'Switch to non-production to change environments' 
                : 'Select which backend to connect to'}
            </p>
          </div>

          <div className="p-2 max-h-[300px] overflow-y-auto">
            {environments.map((env) => (
              <div
                key={env.id}
                className={cn(
                  "flex items-center gap-3 p-2 rounded-md cursor-pointer transition-colors group",
                  activeEnvironment.id === env.id 
                    ? "bg-primary/10" 
                    : "hover:bg-secondary/50",
                  isProduction && env.type !== 'production' && "opacity-50 cursor-not-allowed"
                )}
                onClick={() => !isProduction && handleSelectEnvironment(env.id)}
              >
                <div className={cn(
                  "p-1.5 rounded-md",
                  envTypeConfig[env.type].color
                )}>
                  {envTypeConfig[env.type].icon}
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{env.name}</span>
                    {activeEnvironment.id === env.id && (
                      <Check className="h-3.5 w-3.5 text-primary" />
                    )}
                  </div>
                  <p className="text-[10px] text-muted-foreground truncate">{env.url}</p>
                </div>
                {!env.isDefault && env.type === 'custom' && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity"
                    onClick={(e) => {
                      e.stopPropagation();
                      removeEnvironment(env.id);
                    }}
                  >
                    <Trash2 className="h-3 w-3 text-destructive" />
                  </Button>
                )}
              </div>
            ))}
          </div>

          {!isProduction && (
            <div className="p-2 border-t">
              {showAddForm ? (
                <div className="space-y-3 p-2">
                  <div className="space-y-1.5">
                    <Label className="text-xs">Name</Label>
                    <Input
                      placeholder="My Custom Backend"
                      value={newEnvName}
                      onChange={(e) => setNewEnvName(e.target.value)}
                      className="h-8 text-sm"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <Label className="text-xs">URL</Label>
                    <Input
                      placeholder="http://192.168.1.100:3000"
                      value={newEnvUrl}
                      onChange={(e) => setNewEnvUrl(e.target.value)}
                      className="h-8 text-sm"
                    />
                  </div>
                  <div className="flex gap-2">
                    <Button variant="outline" size="sm" onClick={() => setShowAddForm(false)} className="flex-1">
                      Cancel
                    </Button>
                    <Button size="sm" onClick={handleAddEnvironment} className="flex-1">
                      Add
                    </Button>
                  </div>
                </div>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full justify-start gap-2 text-muted-foreground"
                  onClick={() => setShowAddForm(true)}
                >
                  <Plus className="h-3.5 w-3.5" />
                  Add Custom Environment
                </Button>
              )}
            </div>
          )}

          <div className="p-2 border-t bg-muted/30">
            <p className="text-[10px] text-muted-foreground text-center">
              Current: <code className="px-1 py-0.5 bg-background rounded text-[10px]">{activeEnvironment.url}</code>
            </p>
          </div>
        </PopoverContent>
      </Popover>
    </div>
  );
}