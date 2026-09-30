import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, type RenderResult } from "@testing-library/react";
import type { ReactElement } from "react";
import { createMemoryRouter, RouterProvider, type RouteObject } from "react-router";

/** QueryClient isolado por teste, sem retentativas. */
export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  });
}

/** Renderiza um elemento com QueryClient e roteador em memória. */
export function renderWithProviders(
  ui: ReactElement,
  { route = "/", path = "*", routes }: { route?: string; path?: string; routes?: RouteObject[] } = {},
): RenderResult & { queryClient: QueryClient; router: ReturnType<typeof createMemoryRouter> } {
  const queryClient = createTestQueryClient();
  const router = createMemoryRouter(routes ?? [{ path, element: ui }], { initialEntries: [route] });
  const result = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return { ...result, queryClient, router };
}
