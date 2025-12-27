import React from 'react';
import { Book, Clock, CheckCircle, ArrowRight, Play, Users, Award, Target, Zap } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';

const guides = [
  {
    category: 'Getting Started',
    items: [
      {
        title: 'Quick Start Guide',
        description: 'Set up your first gamification program in under 10 minutes',
        duration: '10 min',
        difficulty: 'Beginner',
        icon: Play,
      },
      {
        title: 'Understanding Programs',
        description: 'Learn how to organize your gamification initiatives into programs',
        duration: '15 min',
        difficulty: 'Beginner',
        icon: Book,
      },
    ],
  },
  {
    category: 'Points & Rewards',
    items: [
      {
        title: 'Setting Up Point Systems',
        description: 'Configure point types, earning rules, and point expiration policies',
        duration: '20 min',
        difficulty: 'Intermediate',
        icon: Zap,
      },
      {
        title: 'Creating Reward Catalogs',
        description: 'Build a compelling rewards catalog to motivate user engagement',
        duration: '25 min',
        difficulty: 'Intermediate',
        icon: Award,
      },
    ],
  },
  {
    category: 'Badges & Achievements',
    items: [
      {
        title: 'Designing Badge Systems',
        description: 'Create meaningful badges that drive specific user behaviors',
        duration: '20 min',
        difficulty: 'Intermediate',
        icon: Award,
      },
      {
        title: 'Achievement Unlocks',
        description: 'Set up automated badge awards based on user actions',
        duration: '15 min',
        difficulty: 'Beginner',
        icon: CheckCircle,
      },
    ],
  },
  {
    category: 'User Engagement',
    items: [
      {
        title: 'Creating Missions',
        description: 'Design multi-step missions to guide user journeys',
        duration: '30 min',
        difficulty: 'Advanced',
        icon: Target,
      },
      {
        title: 'Building Leaderboards',
        description: 'Set up competitive leaderboards to drive engagement',
        duration: '20 min',
        difficulty: 'Intermediate',
        icon: Users,
      },
    ],
  },
];

const difficultyColors: Record<string, string> = {
  Beginner: 'bg-green-500/20 text-green-500',
  Intermediate: 'bg-amber-500/20 text-amber-500',
  Advanced: 'bg-red-500/20 text-red-500',
};

export default function UserGuides() {
  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-3xl font-bold mb-2">User Guides</h1>
        <p className="text-muted-foreground">
          Step-by-step tutorials to help you get the most out of LevelUpOs.
        </p>
      </div>

      {/* Featured Guide */}
      <Card className="bg-gradient-to-br from-primary/10 to-primary/5 border-primary/20">
        <CardContent className="p-6">
          <div className="flex flex-col md:flex-row items-start md:items-center gap-6">
            <div className="w-16 h-16 rounded-2xl bg-primary/20 flex items-center justify-center shrink-0">
              <Play className="w-8 h-8 text-primary" />
            </div>
            <div className="flex-1">
              <Badge className="mb-2">Featured</Badge>
              <h2 className="text-2xl font-bold mb-2">Complete Onboarding Course</h2>
              <p className="text-muted-foreground mb-4">
                Master LevelUpOs from scratch with our comprehensive video course covering all features and best practices.
              </p>
              <div className="flex items-center gap-4 text-sm text-muted-foreground">
                <span className="flex items-center gap-1">
                  <Clock className="w-4 h-4" />
                  2 hours
                </span>
                <span className="flex items-center gap-1">
                  <Book className="w-4 h-4" />
                  12 lessons
                </span>
              </div>
            </div>
            <Button className="shrink-0">
              Start Course
              <ArrowRight className="w-4 h-4 ml-2" />
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Guide Categories */}
      {guides.map((category) => (
        <div key={category.category}>
          <h2 className="text-xl font-semibold mb-4">{category.category}</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {category.items.map((guide) => (
              <Card key={guide.title} className="hover:border-primary/50 hover:shadow-lg transition-all cursor-pointer group">
                <CardContent className="p-6">
                  <div className="flex items-start gap-4">
                    <div className="p-3 rounded-lg bg-secondary/50 group-hover:bg-primary/10 transition-colors">
                      <guide.icon className="w-5 h-5 text-muted-foreground group-hover:text-primary transition-colors" />
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1">
                        <h3 className="font-medium group-hover:text-primary transition-colors">{guide.title}</h3>
                      </div>
                      <p className="text-sm text-muted-foreground mb-3">{guide.description}</p>
                      <div className="flex items-center gap-3">
                        <Badge variant="secondary" className={difficultyColors[guide.difficulty]}>
                          {guide.difficulty}
                        </Badge>
                        <span className="text-xs text-muted-foreground flex items-center gap-1">
                          <Clock className="w-3 h-3" />
                          {guide.duration}
                        </span>
                      </div>
                    </div>
                    <ArrowRight className="w-5 h-5 text-muted-foreground group-hover:text-primary group-hover:translate-x-1 transition-all shrink-0" />
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}