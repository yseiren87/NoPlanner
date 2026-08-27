import { Route, Routes } from "react-router-dom";

import { AppLayout } from "@/components/app-layout";
import { NavigationGuard } from "@/components/navigation-guard";
import { SessionExpiredDialog } from "@/components/session-expired-dialog";
import { HomePage } from "@/pages/home-page";
import { LoginPage } from "@/pages/login-page";
import { NotFoundPage } from "@/pages/not-found-page";
import { ReportPage } from "@/pages/report-page";
import { ProductDesignPage } from "@/pages/product-design-page";
import { AuthCallbackPage } from "@/pages/auth-callback-page";
import { AutonomousPlanningPage } from "@/pages/autonomous-planning-page";
import { AutonomousPlanPage } from "@/pages/autonomous-plan-page";
import { ImplementationPage } from "@/pages/implementation-page";
import { ProjectsPage } from "@/pages/projects-page";

export function App() {
  return (
    <>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/auth/callback" element={<AuthCallbackPage />} />
        <Route element={<NavigationGuard />}>
          <Route element={<AppLayout />}>
            <Route index element={<HomePage />} />
            <Route path="reports/:id" element={<ReportPage />} />
            <Route path="product-designs/:id" element={<ProductDesignPage />} />
            <Route path="planning/new" element={<AutonomousPlanningPage />} />
            <Route path="autonomous-plans/:id" element={<AutonomousPlanPage />} />
            <Route path="implementation" element={<ImplementationPage />} />
            <Route path="projects" element={<ProjectsPage />} />
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Route>
      </Routes>
      <SessionExpiredDialog />
    </>
  );
}
