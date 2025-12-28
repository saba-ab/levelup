import React from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { levels } from '@/lib/mockData';
import { TrendingUp, Users, Sparkles } from 'lucide-react';
import { cn } from '@/lib/utils';
import { AIGenerateDialog } from '@/components/ai/AIGenerateDialog';

export default function Levels() {
  const getTierColor = (tier: string) => {
    switch (tier) {
      case 'bronze': return 'bg-amber-700';
      case 'silver': return 'bg-gray-400';
      case 'gold': return 'bg-yellow-500';
      case 'platinum': return 'bg-slate-300';
      case 'diamond': return 'bg-cyan-400';
      default: return 'bg-primary';
    }
  };

  const totalUsers = levels.reduce((sum, level) => sum + level.usersCount, 0);

  // Connect this to your MySQL backend
  const handleAIGenerate = async (prompt: string): Promise<string> => {
    // Replace with your API call to MySQL backend
    // Example: const response = await fetch('/api/ai/generate', { method: 'POST', body: JSON.stringify({ prompt, type: 'level' }) });
    // return response.json();
    
    // Placeholder for demo
    await new Promise(resolve => setTimeout(resolve, 1500));
    return `Generated Level Description:\n\n"${prompt}"\n\nThis tier rewards dedicated players who have shown consistent engagement. Members enjoy exclusive perks including early access to new features, special badges, and priority support.`;
  };

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Levels & Tiers</h1>
          <p className="text-muted-foreground mt-1">Define progression levels for your users.</p>
        </div>
        <AIGenerateDialog
          trigger={
            <Button className="gap-2">
              <Sparkles className="w-4 h-4" />
              Generate with AI
            </Button>
          }
          title="Generate Level Content"
          placeholder="E.g., Create a description for a Diamond tier level that makes players feel elite..."
          context="Generate level descriptions, tier benefits, or progression milestones"
          onGenerate={handleAIGenerate}
        />
      </div>

      {/* Level Progression Visualization */}
      <Card>
        <CardHeader>
          <CardTitle>Level Progression</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="relative">
            {/* Progress Track */}
            <div className="h-3 bg-secondary rounded-full overflow-hidden">
              <div className="h-full flex">
                {levels.map((level, index) => (
                  <div
                    key={level.id}
                    className={cn("h-full", getTierColor(level.tier))}
                    style={{ width: `${(level.usersCount / totalUsers) * 100}%` }}
                  />
                ))}
              </div>
            </div>
            
            {/* Level Markers */}
            <div className="flex justify-between mt-4">
              {levels.map((level) => (
                <div key={level.id} className="text-center">
                  <div
                    className={cn(
                      "w-12 h-12 rounded-full mx-auto mb-2 flex items-center justify-center",
                      getTierColor(level.tier)
                    )}
                  >
                    <TrendingUp className="w-6 h-6 text-white" />
                  </div>
                  <p className="font-semibold text-sm">{level.name}</p>
                  <p className="text-xs text-muted-foreground">{level.xpThreshold.toLocaleString()} XP</p>
                </div>
              ))}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Levels Table */}
      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Level</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Tier</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">XP Threshold</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Users</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Distribution</th>
                </tr>
              </thead>
              <tbody>
                {levels.map((level) => (
                  <tr key={level.id} className="border-b border-border/50 hover:bg-secondary/30 transition-colors">
                    <td className="p-4">
                      <div className="flex items-center gap-3">
                        <div
                          className={cn(
                            "w-10 h-10 rounded-lg flex items-center justify-center",
                            getTierColor(level.tier)
                          )}
                        >
                          <TrendingUp className="w-5 h-5 text-white" />
                        </div>
                        <span className="font-medium">{level.name}</span>
                      </div>
                    </td>
                    <td className="p-4">
                      <Badge
                        variant="outline"
                        className="capitalize"
                        style={{ borderColor: level.color, color: level.color }}
                      >
                        {level.tier}
                      </Badge>
                    </td>
                    <td className="p-4 font-mono">
                      {level.xpThreshold.toLocaleString()} XP
                    </td>
                    <td className="p-4">
                      <div className="flex items-center gap-2">
                        <Users className="w-4 h-4 text-muted-foreground" />
                        <span>{level.usersCount.toLocaleString()}</span>
                      </div>
                    </td>
                    <td className="p-4">
                      <div className="w-32 h-2 bg-secondary rounded-full overflow-hidden">
                        <div
                          className={cn("h-full", getTierColor(level.tier))}
                          style={{ width: `${(level.usersCount / totalUsers) * 100}%` }}
                        />
                      </div>
                      <span className="text-xs text-muted-foreground">
                        {((level.usersCount / totalUsers) * 100).toFixed(1)}%
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
