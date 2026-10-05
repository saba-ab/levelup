import React, { useState } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { 
  Sparkles, 
  TrendingUp, 
  Award, 
  Gift, 
  Target, 
  GitBranch, 
  Users,
  Loader2,
  Copy,
  Check,
  Wand2,
  Lightbulb,
  Zap
} from 'lucide-react';
import { cn } from '@/lib/utils';

interface Template {
  id: string;
  name: string;
  description: string;
  prompt: string;
  category: string;
}

const templates: Template[] = [
  // Levels
  {
    id: 'level-desc',
    name: 'Level Description',
    description: 'Generate engaging descriptions for player levels',
    prompt: 'Create an engaging description for a {tier} tier level called "{name}" that makes players feel accomplished and motivated to progress.',
    category: 'levels',
  },
  {
    id: 'level-benefits',
    name: 'Tier Benefits',
    description: 'Generate exclusive benefits for each tier',
    prompt: 'List 5 exclusive benefits for {tier} tier members that feel valuable and encourage progression from lower tiers.',
    category: 'levels',
  },
  // Badges
  {
    id: 'badge-idea',
    name: 'Badge Concept',
    description: 'Generate creative badge ideas with names and criteria',
    prompt: 'Create a unique achievement badge for users who {action}. Include a creative name, description, and suggested emoji icon.',
    category: 'badges',
  },
  {
    id: 'badge-collection',
    name: 'Badge Collection',
    description: 'Generate a themed set of related badges',
    prompt: 'Design a collection of 5 themed badges around {theme}. For each badge, provide name, description, unlock criteria, and rarity level.',
    category: 'badges',
  },
  // Rewards
  {
    id: 'reward-catalog',
    name: 'Reward Ideas',
    description: 'Generate reward ideas with point costs',
    prompt: 'Suggest 5 engaging rewards for a {industry} loyalty program. Include reward names, descriptions, suggested point costs, and perceived value.',
    category: 'rewards',
  },
  {
    id: 'reward-tier',
    name: 'Tiered Rewards',
    description: 'Create rewards for different point ranges',
    prompt: 'Create a tiered reward structure with rewards for 500, 1000, 2500, 5000, and 10000 points. Each should feel proportionally valuable.',
    category: 'rewards',
  },
  // Missions
  {
    id: 'mission-onboarding',
    name: 'Onboarding Mission',
    description: 'Create a new user onboarding mission',
    prompt: 'Design an onboarding mission that guides new users through key features. Include 3-5 objectives, estimated completion time, and appropriate XP/badge rewards.',
    category: 'missions',
  },
  {
    id: 'mission-weekly',
    name: 'Weekly Challenge',
    description: 'Generate recurring weekly mission ideas',
    prompt: 'Create an engaging weekly recurring mission for {audience}. Include varied objectives that encourage different types of engagement.',
    category: 'missions',
  },
  // Rules
  {
    id: 'rule-points',
    name: 'Point Rules',
    description: 'Generate point award rules for actions',
    prompt: 'Create a balanced point system for a {type} platform. Define point values for 10 common user actions, ensuring fair progression.',
    category: 'rules',
  },
  {
    id: 'rule-automation',
    name: 'Automation Rules',
    description: 'Create trigger-based automation rules',
    prompt: 'Design 5 automation rules that trigger rewards when users {behavior}. Include trigger conditions, actions, and any cooldown periods.',
    category: 'rules',
  },
  // Segments
  {
    id: 'segment-criteria',
    name: 'Segment Definition',
    description: 'Define user segments with criteria',
    prompt: 'Define a user segment for "{segment_name}" users. Include behavioral criteria, engagement metrics, and suggested personalization strategies.',
    category: 'segments',
  },
  {
    id: 'segment-strategy',
    name: 'Segmentation Strategy',
    description: 'Create a complete segmentation approach',
    prompt: 'Design a user segmentation strategy for a {industry} platform with 5 distinct segments. Include segment names, criteria, and targeted engagement tactics.',
    category: 'segments',
  },
];

const categories = [
  { id: 'all', label: 'All Templates', icon: Sparkles },
  { id: 'levels', label: 'Levels', icon: TrendingUp },
  { id: 'badges', label: 'Badges', icon: Award },
  { id: 'rewards', label: 'Rewards', icon: Gift },
  { id: 'missions', label: 'Missions', icon: Target },
  { id: 'rules', label: 'Rules', icon: GitBranch },
  { id: 'segments', label: 'Segments', icon: Users },
];

export default function AIHub() {
  const [selectedCategory, setSelectedCategory] = useState('all');
  const [selectedTemplate, setSelectedTemplate] = useState<Template | null>(null);
  const [customPrompt, setCustomPrompt] = useState('');
  const [result, setResult] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [copied, setCopied] = useState(false);

  const filteredTemplates = selectedCategory === 'all' 
    ? templates 
    : templates.filter(t => t.category === selectedCategory);

  const handleSelectTemplate = (template: Template) => {
    setSelectedTemplate(template);
    setCustomPrompt(template.prompt);
    setResult('');
  };

  const handleGenerate = async () => {
    if (!customPrompt.trim()) return;
    
    setIsLoading(true);
    setResult('');
    
    try {
      // Connect to your MySQL backend
      // Example: const response = await fetch('/api/ai/generate', { method: 'POST', body: JSON.stringify({ prompt: customPrompt }) });
      
      await new Promise(resolve => setTimeout(resolve, 2000));
      setResult(`AI Generated Content\n${'─'.repeat(40)}\n\nBased on your prompt:\n"${customPrompt.slice(0, 100)}..."\n\n${'─'.repeat(40)}\n\n✨ Generated Result:\n\nThis is a placeholder response. Connect your MySQL backend to get real AI-generated content.\n\nYour backend should:\n1. Receive the prompt from the frontend\n2. Process it through your AI model\n3. Return the generated content\n\nThe response will appear here with proper formatting.`);
    } catch (error) {
      setResult('Error generating content. Please try again.');
    } finally {
      setIsLoading(false);
    }
  };

  const handleCopy = async () => {
    await navigator.clipboard.writeText(result);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const getCategoryIcon = (categoryId: string) => {
    const category = categories.find(c => c.id === categoryId);
    return category ? category.icon : Sparkles;
  };

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col gap-2">
        <div className="flex items-center gap-3">
          <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-primary/20 to-purple-500/20 flex items-center justify-center">
            <Wand2 className="w-6 h-6 text-primary" />
          </div>
          <div>
            <h1 className="text-3xl font-bold">AI Generation Hub</h1>
            <p className="text-muted-foreground">Generate gamification content using AI templates or custom prompts.</p>
          </div>
        </div>
      </div>

      {/* Quick Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <Card className="stat-card">
          <CardContent className="p-4 flex items-center gap-4">
            <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center">
              <Lightbulb className="w-5 h-5 text-primary" />
            </div>
            <div>
              <p className="text-2xl font-bold">{templates.length}</p>
              <p className="text-sm text-muted-foreground">Templates</p>
            </div>
          </CardContent>
        </Card>
        <Card className="stat-card">
          <CardContent className="p-4 flex items-center gap-4">
            <div className="w-10 h-10 rounded-lg bg-green-500/10 flex items-center justify-center">
              <Zap className="w-5 h-5 text-green-500" />
            </div>
            <div>
              <p className="text-2xl font-bold">{categories.length - 1}</p>
              <p className="text-sm text-muted-foreground">Categories</p>
            </div>
          </CardContent>
        </Card>
        <Card className="stat-card">
          <CardContent className="p-4 flex items-center gap-4">
            <div className="w-10 h-10 rounded-lg bg-purple-500/10 flex items-center justify-center">
              <Sparkles className="w-5 h-5 text-purple-500" />
            </div>
            <div>
              <p className="text-2xl font-bold">∞</p>
              <p className="text-sm text-muted-foreground">Custom Prompts</p>
            </div>
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Templates Panel */}
        <div className="lg:col-span-1 space-y-4">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-lg">Templates</CardTitle>
              <CardDescription>Select a template or write your own prompt</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* Category Tabs */}
              <div className="flex flex-wrap gap-2">
                {categories.map((category) => (
                  <Button
                    key={category.id}
                    variant={selectedCategory === category.id ? "default" : "outline"}
                    size="sm"
                    onClick={() => setSelectedCategory(category.id)}
                    className="gap-1.5"
                  >
                    <category.icon className="w-3.5 h-3.5" />
                    {category.id === 'all' ? 'All' : category.label}
                  </Button>
                ))}
              </div>

              {/* Template List */}
              <div className="space-y-2 max-h-[400px] overflow-y-auto pr-1">
                {filteredTemplates.map((template) => {
                  const Icon = getCategoryIcon(template.category);
                  return (
                    <button
                      key={template.id}
                      onClick={() => handleSelectTemplate(template)}
                      className={cn(
                        "w-full text-left p-3 rounded-lg border transition-all",
                        selectedTemplate?.id === template.id
                          ? "border-primary bg-primary/5"
                          : "border-border hover:border-primary/50 hover:bg-secondary/50"
                      )}
                    >
                      <div className="flex items-start gap-3">
                        <div className="w-8 h-8 rounded-md bg-secondary flex items-center justify-center shrink-0">
                          <Icon className="w-4 h-4 text-muted-foreground" />
                        </div>
                        <div className="min-w-0">
                          <p className="font-medium text-sm truncate">{template.name}</p>
                          <p className="text-xs text-muted-foreground line-clamp-2">{template.description}</p>
                        </div>
                      </div>
                    </button>
                  );
                })}
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Generation Panel */}
        <div className="lg:col-span-2 space-y-4">
          <Card>
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="text-lg">
                    {selectedTemplate ? selectedTemplate.name : 'Custom Prompt'}
                  </CardTitle>
                  <CardDescription>
                    {selectedTemplate 
                      ? 'Customize the template prompt or use as-is' 
                      : 'Write your own prompt to generate content'}
                  </CardDescription>
                </div>
                {selectedTemplate && (
                  <Badge variant="outline" className="capitalize">
                    {selectedTemplate.category}
                  </Badge>
                )}
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">Your Prompt</label>
                <Textarea
                  placeholder="Describe what you want to generate..."
                  value={customPrompt}
                  onChange={(e) => setCustomPrompt(e.target.value)}
                  className="min-h-[120px] resize-none"
                />
                <p className="text-xs text-muted-foreground">
                  Tip: Replace placeholders like {'{tier}'} or {'{name}'} with your actual values.
                </p>
              </div>

              <Button 
                onClick={handleGenerate} 
                disabled={isLoading || !customPrompt.trim()}
                className="w-full gap-2"
                size="lg"
              >
                {isLoading ? (
                  <>
                    <Loader2 className="w-4 h-4 animate-spin" />
                    Generating...
                  </>
                ) : (
                  <>
                    <Sparkles className="w-4 h-4" />
                    Generate Content
                  </>
                )}
              </Button>
            </CardContent>
          </Card>

          {/* Result Card */}
          {result && (
            <Card className="animate-fade-in">
              <CardHeader className="pb-3">
                <div className="flex items-center justify-between">
                  <CardTitle className="text-lg flex items-center gap-2">
                    <Sparkles className="w-5 h-5 text-primary" />
                    Generated Result
                  </CardTitle>
                  <Button 
                    variant="ghost" 
                    size="sm" 
                    onClick={handleCopy}
                    className="gap-1.5"
                  >
                    {copied ? (
                      <>
                        <Check className="w-4 h-4" />
                        Copied
                      </>
                    ) : (
                      <>
                        <Copy className="w-4 h-4" />
                        Copy
                      </>
                    )}
                  </Button>
                </div>
              </CardHeader>
              <CardContent>
                <div className="p-4 rounded-lg bg-secondary/50 border min-h-[200px] max-h-[400px] overflow-y-auto whitespace-pre-wrap text-sm font-mono">
                  {result}
                </div>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
