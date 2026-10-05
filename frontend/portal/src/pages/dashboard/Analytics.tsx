import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { TrendingUp, TrendingDown, Users, Zap, Target, Trophy, ArrowUpRight, ArrowDownRight } from "lucide-react";
import {
  LineChart,
  Line,
  BarChart,
  Bar,
  AreaChart,
  Area,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
  Funnel,
  FunnelChart,
  LabelList,
} from "recharts";

// Mock data for analytics
const dailyActiveUsers = [
  { date: "Mar 14", dau: 4200, wau: 12400 },
  { date: "Mar 15", dau: 4350, wau: 12600 },
  { date: "Mar 16", dau: 4100, wau: 12300 },
  { date: "Mar 17", dau: 4500, wau: 12800 },
  { date: "Mar 18", dau: 4800, wau: 13200 },
  { date: "Mar 19", dau: 5100, wau: 13800 },
  { date: "Mar 20", dau: 5300, wau: 14200 },
];

const engagementMetrics = [
  { metric: "Avg Session Duration", value: "8m 42s", change: 12.5, trend: "up" },
  { metric: "Events per User", value: "14.2", change: 8.3, trend: "up" },
  { metric: "Mission Completion", value: "67%", change: -2.1, trend: "down" },
  { metric: "Badge Unlock Rate", value: "23%", change: 5.7, trend: "up" },
];

const missionCompletionData = [
  { name: "Welcome Journey", started: 4500, completed: 3015, rate: 67 },
  { name: "Weekly Warrior", started: 2800, completed: 1260, rate: 45 },
  { name: "Power User Path", started: 1200, completed: 480, rate: 40 },
  { name: "Social Champion", started: 890, completed: 356, rate: 40 },
];

const badgeVelocity = [
  { date: "Week 1", unlocks: 1200 },
  { date: "Week 2", unlocks: 1450 },
  { date: "Week 3", unlocks: 1680 },
  { date: "Week 4", unlocks: 1890 },
];

const cohortData = [
  { cohort: "Jan 2024", week0: 100, week1: 72, week2: 58, week3: 45, week4: 38, week5: 32, week6: 28, week7: 25, week8: 23 },
  { cohort: "Feb 2024", week0: 100, week1: 75, week2: 62, week3: 51, week4: 44, week5: 38, week6: 33, week7: 29, week8: null },
  { cohort: "Mar 2024", week0: 100, week1: 78, week2: 65, week3: 55, week4: 48, week5: 42, week6: null, week7: null, week8: null },
];

const retentionCurve = [
  { week: "Week 0", retention: 100 },
  { week: "Week 1", retention: 75 },
  { week: "Week 2", retention: 62 },
  { week: "Week 3", retention: 51 },
  { week: "Week 4", retention: 44 },
  { week: "Week 5", retention: 38 },
  { week: "Week 6", retention: 33 },
  { week: "Week 7", retention: 29 },
  { week: "Week 8", retention: 26 },
];

const rulesPerformance = [
  { name: "First Purchase Bonus", activations: 4520, pointsAwarded: 678000, conversion: 34 },
  { name: "Daily Login Streak", activations: 3890, pointsAwarded: 194500, conversion: 78 },
  { name: "Signup Welcome", activations: 2340, pointsAwarded: 234000, conversion: 92 },
  { name: "Referral Reward", activations: 1890, pointsAwarded: 378000, conversion: 45 },
  { name: "Review Bonus", activations: 1230, pointsAwarded: 123000, conversion: 28 },
];

const conversionFunnel = [
  { name: "Visitors", value: 10000, fill: "hsl(var(--primary))" },
  { name: "Signups", value: 4500, fill: "hsl(var(--chart-2))" },
  { name: "First Action", value: 3200, fill: "hsl(var(--chart-3))" },
  { name: "Engaged", value: 1800, fill: "hsl(var(--chart-4))" },
  { name: "Power Users", value: 450, fill: "hsl(var(--chart-5))" },
];

const pointsDistribution = [
  { range: "0-100", users: 2500 },
  { range: "101-500", users: 3200 },
  { range: "501-1K", users: 1800 },
  { range: "1K-5K", users: 890 },
  { range: "5K-10K", users: 340 },
  { range: "10K+", users: 120 },
];

const eventsVolume = [
  { date: "Mar 14", events: 12400, points: 186000 },
  { date: "Mar 15", events: 14200, points: 213000 },
  { date: "Mar 16", events: 13800, points: 207000 },
  { date: "Mar 17", events: 15600, points: 234000 },
  { date: "Mar 18", events: 18200, points: 273000 },
  { date: "Mar 19", events: 16800, points: 252000 },
  { date: "Mar 20", events: 14500, points: 217500 },
];

const StatCard = ({ title, value, change, trend, icon: Icon }: {
  title: string;
  value: string;
  change?: number;
  trend?: "up" | "down";
  icon: React.ElementType;
}) => (
  <Card className="border-border/50 bg-card/50 backdrop-blur">
    <CardContent className="p-6">
      <div className="flex items-center justify-between">
        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">{title}</p>
          <p className="text-2xl font-bold text-foreground">{value}</p>
          {change !== undefined && (
            <div className={`flex items-center gap-1 text-sm ${trend === "up" ? "text-green-500" : "text-red-500"}`}>
              {trend === "up" ? <ArrowUpRight className="h-4 w-4" /> : <ArrowDownRight className="h-4 w-4" />}
              <span>{Math.abs(change)}%</span>
              <span className="text-muted-foreground">vs last week</span>
            </div>
          )}
        </div>
        <div className="p-3 rounded-xl bg-primary/10">
          <Icon className="h-6 w-6 text-primary" />
        </div>
      </div>
    </CardContent>
  </Card>
);

const CohortTable = () => (
  <div className="overflow-x-auto">
    <table className="w-full text-sm">
      <thead>
        <tr className="border-b border-border">
          <th className="text-left py-3 px-4 font-medium text-muted-foreground">Cohort</th>
          {["Week 0", "Week 1", "Week 2", "Week 3", "Week 4", "Week 5", "Week 6", "Week 7", "Week 8"].map((week) => (
            <th key={week} className="text-center py-3 px-3 font-medium text-muted-foreground">{week}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {cohortData.map((row, idx) => (
          <tr key={idx} className="border-b border-border/50">
            <td className="py-3 px-4 font-medium text-foreground">{row.cohort}</td>
            {[row.week0, row.week1, row.week2, row.week3, row.week4, row.week5, row.week6, row.week7, row.week8].map((val, i) => (
              <td key={i} className="text-center py-3 px-3">
                {val !== null ? (
                  <div
                    className="mx-auto w-12 py-1 rounded text-xs font-medium"
                    style={{
                      backgroundColor: `hsl(var(--primary) / ${val / 100})`,
                      color: val > 50 ? "white" : "hsl(var(--foreground))",
                    }}
                  >
                    {val}%
                  </div>
                ) : (
                  <span className="text-muted-foreground">-</span>
                )}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  </div>
);

export default function Analytics() {
  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold text-foreground">Analytics</h1>
          <p className="text-muted-foreground mt-1">Track performance, engagement, and retention metrics</p>
        </div>
        <Select defaultValue="7d">
          <SelectTrigger className="w-[180px]">
            <SelectValue placeholder="Select period" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="24h">Last 24 hours</SelectItem>
            <SelectItem value="7d">Last 7 days</SelectItem>
            <SelectItem value="30d">Last 30 days</SelectItem>
            <SelectItem value="90d">Last 90 days</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="overview" className="space-y-6">
        <TabsList className="bg-muted/50 p-1">
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="engagement">Engagement</TabsTrigger>
          <TabsTrigger value="retention">Retention</TabsTrigger>
          <TabsTrigger value="rules">Rules Performance</TabsTrigger>
        </TabsList>

        {/* Overview Tab */}
        <TabsContent value="overview" className="space-y-6">
          {/* Key Metrics */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <StatCard title="Total Events" value="105.5K" change={12.3} trend="up" icon={Zap} />
            <StatCard title="Active Users" value="5,342" change={8.7} trend="up" icon={Users} />
            <StatCard title="Points Awarded" value="1.58M" change={15.2} trend="up" icon={Trophy} />
            <StatCard title="Mission Completions" value="2,847" change={-3.4} trend="down" icon={Target} />
          </div>

          {/* Charts Row */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* Events Volume */}
            <Card className="border-border/50 bg-card/50 backdrop-blur">
              <CardHeader>
                <CardTitle className="text-lg">Events & Points Volume</CardTitle>
                <CardDescription>Daily events and points awarded over time</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="h-[300px]">
                  <ResponsiveContainer width="100%" height="100%">
                    <AreaChart data={eventsVolume}>
                      <defs>
                        <linearGradient id="eventsGradient" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="5%" stopColor="hsl(var(--primary))" stopOpacity={0.3} />
                          <stop offset="95%" stopColor="hsl(var(--primary))" stopOpacity={0} />
                        </linearGradient>
                      </defs>
                      <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
                      <XAxis dataKey="date" stroke="hsl(var(--muted-foreground))" fontSize={12} />
                      <YAxis stroke="hsl(var(--muted-foreground))" fontSize={12} />
                      <Tooltip
                        contentStyle={{
                          backgroundColor: "hsl(var(--card))",
                          border: "1px solid hsl(var(--border))",
                          borderRadius: "8px",
                        }}
                      />
                      <Area
                        type="monotone"
                        dataKey="events"
                        stroke="hsl(var(--primary))"
                        fill="url(#eventsGradient)"
                        strokeWidth={2}
                      />
                    </AreaChart>
                  </ResponsiveContainer>
                </div>
              </CardContent>
            </Card>

            {/* Points Distribution */}
            <Card className="border-border/50 bg-card/50 backdrop-blur">
              <CardHeader>
                <CardTitle className="text-lg">Points Distribution</CardTitle>
                <CardDescription>User distribution by XP range</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="h-[300px]">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={pointsDistribution}>
                      <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
                      <XAxis dataKey="range" stroke="hsl(var(--muted-foreground))" fontSize={12} />
                      <YAxis stroke="hsl(var(--muted-foreground))" fontSize={12} />
                      <Tooltip
                        contentStyle={{
                          backgroundColor: "hsl(var(--card))",
                          border: "1px solid hsl(var(--border))",
                          borderRadius: "8px",
                        }}
                      />
                      <Bar dataKey="users" fill="hsl(var(--primary))" radius={[4, 4, 0, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Sparkline Cards */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            {engagementMetrics.map((metric, idx) => (
              <Card key={idx} className="border-border/50 bg-card/50 backdrop-blur">
                <CardContent className="p-4">
                  <p className="text-sm text-muted-foreground">{metric.metric}</p>
                  <div className="flex items-baseline gap-2 mt-1">
                    <span className="text-2xl font-bold text-foreground">{metric.value}</span>
                    <span className={`text-sm flex items-center gap-0.5 ${metric.trend === "up" ? "text-green-500" : "text-red-500"}`}>
                      {metric.trend === "up" ? <TrendingUp className="h-3 w-3" /> : <TrendingDown className="h-3 w-3" />}
                      {metric.change}%
                    </span>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </TabsContent>

        {/* Engagement Tab */}
        <TabsContent value="engagement" className="space-y-6">
          {/* DAU/WAU Chart */}
          <Card className="border-border/50 bg-card/50 backdrop-blur">
            <CardHeader>
              <CardTitle className="text-lg">Daily & Weekly Active Users</CardTitle>
              <CardDescription>User activity trends over the past week</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="h-[350px]">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={dailyActiveUsers}>
                    <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
                    <XAxis dataKey="date" stroke="hsl(var(--muted-foreground))" fontSize={12} />
                    <YAxis stroke="hsl(var(--muted-foreground))" fontSize={12} />
                    <Tooltip
                      contentStyle={{
                        backgroundColor: "hsl(var(--card))",
                        border: "1px solid hsl(var(--border))",
                        borderRadius: "8px",
                      }}
                    />
                    <Legend />
                    <Line
                      type="monotone"
                      dataKey="dau"
                      stroke="hsl(var(--primary))"
                      strokeWidth={2}
                      dot={{ fill: "hsl(var(--primary))" }}
                      name="Daily Active"
                    />
                    <Line
                      type="monotone"
                      dataKey="wau"
                      stroke="hsl(var(--chart-2))"
                      strokeWidth={2}
                      dot={{ fill: "hsl(var(--chart-2))" }}
                      name="Weekly Active"
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </CardContent>
          </Card>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* Mission Completion Rates */}
            <Card className="border-border/50 bg-card/50 backdrop-blur">
              <CardHeader>
                <CardTitle className="text-lg">Mission Completion Rates</CardTitle>
                <CardDescription>Started vs completed missions</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  {missionCompletionData.map((mission, idx) => (
                    <div key={idx} className="space-y-2">
                      <div className="flex items-center justify-between text-sm">
                        <span className="font-medium text-foreground">{mission.name}</span>
                        <span className="text-muted-foreground">{mission.rate}%</span>
                      </div>
                      <div className="h-2 bg-muted rounded-full overflow-hidden">
                        <div
                          className="h-full bg-primary rounded-full transition-all"
                          style={{ width: `${mission.rate}%` }}
                        />
                      </div>
                      <div className="flex justify-between text-xs text-muted-foreground">
                        <span>{mission.started.toLocaleString()} started</span>
                        <span>{mission.completed.toLocaleString()} completed</span>
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>

            {/* Badge Unlock Velocity */}
            <Card className="border-border/50 bg-card/50 backdrop-blur">
              <CardHeader>
                <CardTitle className="text-lg">Badge Unlock Velocity</CardTitle>
                <CardDescription>Weekly badge unlocks trend</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="h-[280px]">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={badgeVelocity}>
                      <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
                      <XAxis dataKey="date" stroke="hsl(var(--muted-foreground))" fontSize={12} />
                      <YAxis stroke="hsl(var(--muted-foreground))" fontSize={12} />
                      <Tooltip
                        contentStyle={{
                          backgroundColor: "hsl(var(--card))",
                          border: "1px solid hsl(var(--border))",
                          borderRadius: "8px",
                        }}
                      />
                      <Bar dataKey="unlocks" fill="hsl(var(--chart-3))" radius={[4, 4, 0, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        {/* Retention Tab */}
        <TabsContent value="retention" className="space-y-6">
          {/* Cohort Retention Table */}
          <Card className="border-border/50 bg-card/50 backdrop-blur">
            <CardHeader>
              <CardTitle className="text-lg">Cohort Retention Analysis</CardTitle>
              <CardDescription>User retention by signup cohort (percentage returning each week)</CardDescription>
            </CardHeader>
            <CardContent>
              <CohortTable />
            </CardContent>
          </Card>

          {/* Retention Curve */}
          <Card className="border-border/50 bg-card/50 backdrop-blur">
            <CardHeader>
              <CardTitle className="text-lg">Retention Curve</CardTitle>
              <CardDescription>Average user retention over time</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="h-[350px]">
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={retentionCurve}>
                    <defs>
                      <linearGradient id="retentionGradient" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor="hsl(var(--chart-4))" stopOpacity={0.4} />
                        <stop offset="95%" stopColor="hsl(var(--chart-4))" stopOpacity={0} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
                    <XAxis dataKey="week" stroke="hsl(var(--muted-foreground))" fontSize={12} />
                    <YAxis stroke="hsl(var(--muted-foreground))" fontSize={12} unit="%" />
                    <Tooltip
                      contentStyle={{
                        backgroundColor: "hsl(var(--card))",
                        border: "1px solid hsl(var(--border))",
                        borderRadius: "8px",
                      }}
                      formatter={(value: number) => [`${value}%`, "Retention"]}
                    />
                    <Area
                      type="monotone"
                      dataKey="retention"
                      stroke="hsl(var(--chart-4))"
                      fill="url(#retentionGradient)"
                      strokeWidth={2}
                    />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            </CardContent>
          </Card>

          {/* Key Retention Metrics */}
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <Card className="border-border/50 bg-card/50 backdrop-blur">
              <CardContent className="p-6 text-center">
                <p className="text-sm text-muted-foreground">Day 1 Retention</p>
                <p className="text-3xl font-bold text-foreground mt-2">75%</p>
                <Badge variant="outline" className="mt-2 text-green-500 border-green-500/30">
                  <TrendingUp className="h-3 w-3 mr-1" /> +3.2%
                </Badge>
              </CardContent>
            </Card>
            <Card className="border-border/50 bg-card/50 backdrop-blur">
              <CardContent className="p-6 text-center">
                <p className="text-sm text-muted-foreground">Day 7 Retention</p>
                <p className="text-3xl font-bold text-foreground mt-2">44%</p>
                <Badge variant="outline" className="mt-2 text-green-500 border-green-500/30">
                  <TrendingUp className="h-3 w-3 mr-1" /> +1.8%
                </Badge>
              </CardContent>
            </Card>
            <Card className="border-border/50 bg-card/50 backdrop-blur">
              <CardContent className="p-6 text-center">
                <p className="text-sm text-muted-foreground">Day 30 Retention</p>
                <p className="text-3xl font-bold text-foreground mt-2">26%</p>
                <Badge variant="outline" className="mt-2 text-red-500 border-red-500/30">
                  <TrendingDown className="h-3 w-3 mr-1" /> -0.5%
                </Badge>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        {/* Rules Performance Tab */}
        <TabsContent value="rules" className="space-y-6">
          {/* Conversion Funnel */}
          <Card className="border-border/50 bg-card/50 backdrop-blur">
            <CardHeader>
              <CardTitle className="text-lg">Conversion Funnel</CardTitle>
              <CardDescription>User progression through gamification stages</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-col md:flex-row items-center gap-6">
                {conversionFunnel.map((stage, idx) => (
                  <div key={idx} className="flex-1 text-center">
                    <div
                      className="mx-auto rounded-lg p-6 transition-all hover:scale-105"
                      style={{
                        backgroundColor: stage.fill,
                        width: `${100 - idx * 15}%`,
                        minWidth: "100px",
                      }}
                    >
                      <p className="text-2xl font-bold text-white">{stage.value.toLocaleString()}</p>
                    </div>
                    <p className="mt-2 text-sm font-medium text-foreground">{stage.name}</p>
                    {idx > 0 && (
                      <p className="text-xs text-muted-foreground">
                        {((stage.value / conversionFunnel[idx - 1].value) * 100).toFixed(1)}% conversion
                      </p>
                    )}
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          {/* Rules Performance Table */}
          <Card className="border-border/50 bg-card/50 backdrop-blur">
            <CardHeader>
              <CardTitle className="text-lg">Rules by Performance</CardTitle>
              <CardDescription>Activation counts, points awarded, and conversion rates</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-border">
                      <th className="text-left py-3 px-4 font-medium text-muted-foreground">Rule Name</th>
                      <th className="text-right py-3 px-4 font-medium text-muted-foreground">Activations</th>
                      <th className="text-right py-3 px-4 font-medium text-muted-foreground">Points Awarded</th>
                      <th className="text-right py-3 px-4 font-medium text-muted-foreground">Conversion</th>
                      <th className="text-left py-3 px-4 font-medium text-muted-foreground">Performance</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rulesPerformance.map((rule, idx) => (
                      <tr key={idx} className="border-b border-border/50 hover:bg-muted/30 transition-colors">
                        <td className="py-4 px-4 font-medium text-foreground">{rule.name}</td>
                        <td className="py-4 px-4 text-right text-foreground">{rule.activations.toLocaleString()}</td>
                        <td className="py-4 px-4 text-right text-foreground">{rule.pointsAwarded.toLocaleString()}</td>
                        <td className="py-4 px-4 text-right">
                          <Badge variant={rule.conversion >= 50 ? "default" : "secondary"}>
                            {rule.conversion}%
                          </Badge>
                        </td>
                        <td className="py-4 px-4">
                          <div className="w-24 h-2 bg-muted rounded-full overflow-hidden">
                            <div
                              className="h-full bg-primary rounded-full"
                              style={{ width: `${rule.conversion}%` }}
                            />
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>

          {/* Rules Charts */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* Activations by Rule */}
            <Card className="border-border/50 bg-card/50 backdrop-blur">
              <CardHeader>
                <CardTitle className="text-lg">Activations by Rule</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="h-[300px]">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={rulesPerformance} layout="vertical">
                      <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
                      <XAxis type="number" stroke="hsl(var(--muted-foreground))" fontSize={12} />
                      <YAxis dataKey="name" type="category" stroke="hsl(var(--muted-foreground))" fontSize={11} width={120} />
                      <Tooltip
                        contentStyle={{
                          backgroundColor: "hsl(var(--card))",
                          border: "1px solid hsl(var(--border))",
                          borderRadius: "8px",
                        }}
                      />
                      <Bar dataKey="activations" fill="hsl(var(--primary))" radius={[0, 4, 4, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </CardContent>
            </Card>

            {/* Points by Rule */}
            <Card className="border-border/50 bg-card/50 backdrop-blur">
              <CardHeader>
                <CardTitle className="text-lg">Points Awarded by Rule</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="h-[300px]">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={rulesPerformance} layout="vertical">
                      <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
                      <XAxis type="number" stroke="hsl(var(--muted-foreground))" fontSize={12} />
                      <YAxis dataKey="name" type="category" stroke="hsl(var(--muted-foreground))" fontSize={11} width={120} />
                      <Tooltip
                        contentStyle={{
                          backgroundColor: "hsl(var(--card))",
                          border: "1px solid hsl(var(--border))",
                          borderRadius: "8px",
                        }}
                      />
                      <Bar dataKey="pointsAwarded" fill="hsl(var(--chart-2))" radius={[0, 4, 4, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
}
