/**
 * Code-based TanStack Router tree (createRootRoute/createRoute rather than
 * the file-based codegen plugin — no extra build-time dependency beyond the
 * pinned VERSIONS.md set, and the tree is small enough to read in one file).
 *
 *   /login          public — API key sign-in
 *   /onboarding     public — 4-step self-service setup
 *   (authed layout) guard: no stored key -> redirect /login
 *     /             dashboard
 *     /calls        list + typed search filters
 *     /calls/$callId detail
 *     /messages     structured messages
 *     /appointments appointments
 *     /settings     mode toggle, hours, coverage, numbers
 */
import {
  Outlet,
  createRootRoute,
  createRoute,
  createRouter,
  redirect,
} from "@tanstack/react-router";

import { Layout } from "./components/Layout";
import { AppointmentsPage } from "./pages/AppointmentsPage";
import { CallDetailPage } from "./pages/CallDetailPage";
import { CallsPage, callsSearchSchema } from "./pages/CallsPage";
import { DashboardPage } from "./pages/DashboardPage";
import { LoginPage } from "./pages/LoginPage";
import { MessagesPage } from "./pages/MessagesPage";
import { OnboardingPage } from "./pages/OnboardingPage";
import { SettingsPage } from "./pages/SettingsPage";

const rootRoute = createRootRoute({
  component: Outlet,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  component: LoginPage,
});

const onboardingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/onboarding",
  component: OnboardingPage,
});

/**
 * Pathless layout route guarding everything behind the API key. The guard
 * reads localStorage directly so it works during the initial navigation,
 * before React context exists.
 */
const authedRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "authed",
  beforeLoad: () => {
    if (localStorage.getItem("frontdesk.apiKey") === null) {
      throw redirect({ to: "/login" });
    }
  },
  component: Layout,
});

const dashboardRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/",
  component: DashboardPage,
});

const callsRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/calls",
  validateSearch: callsSearchSchema,
  component: CallsPage,
});

const callDetailRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/calls/$callId",
  component: CallDetailPage,
});

const messagesRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/messages",
  component: MessagesPage,
});

const appointmentsRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/appointments",
  component: AppointmentsPage,
});

const settingsRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/settings",
  component: SettingsPage,
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  onboardingRoute,
  authedRoute.addChildren([
    dashboardRoute,
    callsRoute,
    callDetailRoute,
    messagesRoute,
    appointmentsRoute,
    settingsRoute,
  ]),
]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
