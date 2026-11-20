import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@fontsource-variable/red-hat-text";
import "@fontsource-variable/red-hat-mono";

import { ApiError } from "./api/client";
import { App } from "./App";
import "./styles/tokens.css";
import "./styles/base.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000,
      // A 4xx will not change on retry; network blips and 5xx might.
      retry: (failures, error) => !(error instanceof ApiError && error.status >= 400 && error.status < 500) && failures < 2,
    },
  },
});

const root = document.getElementById("root");
if (!root) throw new Error("missing #root element");

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
);
