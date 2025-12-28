import { Toaster } from "@/components/ui/toaster";
import { Toaster as Sonner } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route } from "react-router-dom";
import { AuthProvider } from "@/contexts/AuthContext";
import { ThemeProvider } from "@/contexts/ThemeContext";

import Login from "./pages/Login";
import Register from "./pages/Register";
import ForgotPassword from "./pages/ForgotPassword";
import DashboardLayout from "./components/layout/DashboardLayout";
import Overview from "./pages/dashboard/Overview";
import Programs from "./pages/dashboard/Programs";
import Rules from "./pages/dashboard/Rules";
import RuleBuilder from "./pages/dashboard/RuleBuilder";
import Badges from "./pages/dashboard/mechanics/Badges";
import Levels from "./pages/dashboard/mechanics/Levels";
import Leaderboards from "./pages/dashboard/mechanics/Leaderboards";
import Points from "./pages/dashboard/mechanics/Points";
import Missions from "./pages/dashboard/mechanics/Missions";
import Streaks from "./pages/dashboard/mechanics/Streaks";
import Rewards from "./pages/dashboard/mechanics/Rewards";
import Players from "./pages/dashboard/Players";
import PlayerProfile from "./pages/dashboard/PlayerProfile";
import Analytics from "./pages/dashboard/Analytics";
import Integrations from "./pages/dashboard/Integrations";
import AuditLogs from "./pages/dashboard/AuditLogs";
import Segments from "./pages/dashboard/Segments";
import Notifications from "./pages/dashboard/Notifications";
import Settings from "./pages/dashboard/Settings";
import DocsOverview from "./pages/dashboard/docs/DocsOverview";
import ApiReference from "./pages/dashboard/docs/ApiReference";
import UserGuides from "./pages/dashboard/docs/UserGuides";
import DeveloperDocs from "./pages/dashboard/docs/DeveloperDocs";
import NotFound from "./pages/NotFound";

const queryClient = new QueryClient();

const App = () => (
  <QueryClientProvider client={queryClient}>
    <ThemeProvider>
      <AuthProvider>
        <TooltipProvider>
          <Toaster />
          <Sonner />
          <BrowserRouter>
            <Routes>
              {/* Auth Routes */}
              <Route path="/login" element={<Login />} />
              <Route path="/register" element={<Register />} />
              <Route path="/forgot-password" element={<ForgotPassword />} />
              
              {/* Dashboard Routes */}
              <Route path="/" element={<DashboardLayout />}>
                <Route index element={<Overview />} />
                <Route path="programs" element={<Programs />} />
                <Route path="rules" element={<Rules />} />
                <Route path="rules/new" element={<RuleBuilder />} />
                <Route path="mechanics/points" element={<Points />} />
                <Route path="mechanics/badges" element={<Badges />} />
                <Route path="mechanics/levels" element={<Levels />} />
                <Route path="mechanics/missions" element={<Missions />} />
                <Route path="mechanics/streaks" element={<Streaks />} />
                <Route path="mechanics/leaderboards" element={<Leaderboards />} />
                <Route path="mechanics/rewards" element={<Rewards />} />
                <Route path="players" element={<Players />} />
                <Route path="players/:playerId" element={<PlayerProfile />} />
                <Route path="analytics" element={<Analytics />} />
                <Route path="integrations" element={<Integrations />} />
                <Route path="audit-logs" element={<AuditLogs />} />
                <Route path="segments" element={<Segments />} />
                <Route path="notifications" element={<Notifications />} />
                <Route path="docs" element={<DocsOverview />} />
                <Route path="docs/api" element={<ApiReference />} />
                <Route path="docs/guides" element={<UserGuides />} />
                <Route path="docs/developer" element={<DeveloperDocs />} />
                <Route path="settings" element={<Settings />} />
              </Route>
              
              <Route path="*" element={<NotFound />} />
            </Routes>
          </BrowserRouter>
        </TooltipProvider>
      </AuthProvider>
    </ThemeProvider>
  </QueryClientProvider>
);

export default App;
