import { createBrowserRouter, isRouteErrorResponse, RouterProvider, useRouteError } from "react-router";

import { AppShell } from "./components/AppShell";
import { EmptyState } from "./components/EmptyState";
import { BatchPage } from "./pages/BatchPage";
import { ComposePage } from "./pages/ComposePage";
import { NotFoundPage } from "./pages/NotFoundPage";
import { NotificationsPage } from "./pages/NotificationsPage";
import { OverviewPage } from "./pages/OverviewPage";
import { ProviderPage } from "./pages/ProviderPage";
import { TemplatesPage } from "./pages/TemplatesPage";

function RouteError() {
  const error = useRouteError();
  const message = isRouteErrorResponse(error) ? error.statusText : error instanceof Error ? error.message : "Unknown error";
  return (
    <main style={{ padding: 24 }}>
      <EmptyState title="This screen failed to load" action={<a href="/">Reload the console</a>}>
        {message}
      </EmptyState>
    </main>
  );
}

const router = createBrowserRouter([
  {
    element: <AppShell />,
    errorElement: <RouteError />,
    children: [
      { index: true, element: <OverviewPage /> },
      { path: "notifications", element: <NotificationsPage /> },
      { path: "batches/:id", element: <BatchPage /> },
      { path: "compose", element: <ComposePage /> },
      { path: "templates", element: <TemplatesPage /> },
      { path: "provider", element: <ProviderPage /> },
      { path: "*", element: <NotFoundPage /> },
    ],
  },
]);

export function App() {
  return <RouterProvider router={router} />;
}
