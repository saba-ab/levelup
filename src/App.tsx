import { Toaster } from "@/components/ui/toaster";
import { Toaster as Sonner } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route } from "react-router-dom";
import { AuthProvider } from "@/contexts/AuthContext";
import { ThemeProvider } from "@/contexts/ThemeContext";
import ProtectedRoute from "@/components/ProtectedRoute";

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
import PlayerComparison from "./pages/dashboard/PlayerComparison";
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
import AIHub from "./pages/dashboard/AIHub";
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
              
              {/* Dashboard Routes - Protected */}
              <Route path="/" element={<DashboardLayout />}>
                <Route index element={<ProtectedRoute permission="view:overview"><Overview /></ProtectedRoute>} />
                <Route path="ai-hub" element={<ProtectedRoute permission="view:ai-hub"><AIHub /></ProtectedRoute>} />
                <Route path="programs" element={<ProtectedRoute permission="view:programs"><Programs /></ProtectedRoute>} />
                <Route path="rules" element={<ProtectedRoute permission="view:rules"><Rules /></ProtectedRoute>} />
                <Route path="rules/new" element={<ProtectedRoute permission="manage:rules"><RuleBuilder /></ProtectedRoute>} />
                <Route path="mechanics/points" element={<ProtectedRoute permission="view:mechanics"><Points /></ProtectedRoute>} />
                <Route path="mechanics/badges" element={<ProtectedRoute permission="view:mechanics"><Badges /></ProtectedRoute>} />
                <Route path="mechanics/levels" element={<ProtectedRoute permission="view:mechanics"><Levels /></ProtectedRoute>} />
                <Route path="mechanics/missions" element={<ProtectedRoute permission="view:mechanics"><Missions /></ProtectedRoute>} />
                <Route path="mechanics/streaks" element={<ProtectedRoute permission="view:mechanics"><Streaks /></ProtectedRoute>} />
                <Route path="mechanics/leaderboards" element={<ProtectedRoute permission="view:mechanics"><Leaderboards /></ProtectedRoute>} />
                <Route path="mechanics/rewards" element={<ProtectedRoute permission="view:mechanics"><Rewards /></ProtectedRoute>} />
                <Route path="players" element={<ProtectedRoute permission="view:players"><Players /></ProtectedRoute>} />
                <Route path="players/compare" element={<ProtectedRoute permission="view:players"><PlayerComparison /></ProtectedRoute>} />
                <Route path="players/:playerId" element={<ProtectedRoute permission="view:players"><PlayerProfile /></ProtectedRoute>} />
                <Route path="analytics" element={<ProtectedRoute permission="view:analytics"><Analytics /></ProtectedRoute>} />
                <Route path="integrations" element={<ProtectedRoute permission="view:integrations"><Integrations /></ProtectedRoute>} />
                <Route path="audit-logs" element={<ProtectedRoute permission="view:audit-logs"><AuditLogs /></ProtectedRoute>} />
                <Route path="segments" element={<ProtectedRoute permission="view:segments"><Segments /></ProtectedRoute>} />
                <Route path="notifications" element={<ProtectedRoute permission="view:notifications"><Notifications /></ProtectedRoute>} />
                <Route path="docs" element={<ProtectedRoute permission="view:docs"><DocsOverview /></ProtectedRoute>} />
                <Route path="docs/api" element={<ProtectedRoute permission="view:docs"><ApiReference /></ProtectedRoute>} />
                <Route path="docs/guides" element={<ProtectedRoute permission="view:docs"><UserGuides /></ProtectedRoute>} />
                <Route path="docs/developer" element={<ProtectedRoute permission="view:docs"><DeveloperDocs /></ProtectedRoute>} />
                <Route path="settings" element={<ProtectedRoute permission="view:settings"><Settings /></ProtectedRoute>} />
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
