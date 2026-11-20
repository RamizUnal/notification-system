import {
  type InfiniteData,
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import {
  api,
  call,
  type Channel,
  type NotificationInput,
  type NotificationPage,
  type Priority,
  type Status,
  type TemplateInput,
} from "./client";

/** Filters for notification listings; mirrors the API's query parameters. */
export interface ListFilter {
  status?: Status[];
  channel?: Channel[];
  priority?: Priority[];
  recipient?: string;
  batch_id?: string;
  created_from?: string;
  created_to?: string;
  limit?: number;
}

export const keys = {
  metrics: (window: string) => ["metrics", window] as const,
  lists: () => ["notifications", "list"] as const,
  list: (filter: ListFilter) => ["notifications", "list", filter] as const,
  notification: (id: string) => ["notifications", "detail", id] as const,
  attempts: (id: string) => ["notifications", "attempts", id] as const,
  batch: (id: string) => ["batches", id] as const,
  templates: () => ["templates"] as const,
};

export type NotificationPages = InfiniteData<NotificationPage, string | undefined>;

export function useMetrics(window: string) {
  return useQuery({
    queryKey: keys.metrics(window),
    queryFn: () => call(api.GET("/api/v1/metrics", { params: { query: { window } } })),
    refetchInterval: 2000,
    placeholderData: keepPreviousData,
  });
}

export function useNotifications(filter: ListFilter) {
  return useInfiniteQuery({
    queryKey: keys.list(filter),
    queryFn: ({ pageParam }) =>
      call(api.GET("/api/v1/notifications", { params: { query: { ...filter, cursor: pageParam } } })),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    placeholderData: keepPreviousData,
  });
}

export function useNotification(id: string | undefined) {
  return useQuery({
    queryKey: keys.notification(id ?? ""),
    queryFn: () => call(api.GET("/api/v1/notifications/{id}", { params: { path: { id: id ?? "" } } })),
    enabled: Boolean(id),
  });
}

export function useAttempts(id: string | undefined) {
  return useQuery({
    queryKey: keys.attempts(id ?? ""),
    queryFn: () =>
      call(api.GET("/api/v1/notifications/{id}/attempts", { params: { path: { id: id ?? "" } } })),
    enabled: Boolean(id),
  });
}

export function useBatch(id: string | undefined) {
  return useQuery({
    queryKey: keys.batch(id ?? ""),
    queryFn: () => call(api.GET("/api/v1/batches/{id}", { params: { path: { id: id ?? "" } } })),
    enabled: Boolean(id),
  });
}

export function useTemplates() {
  return useQuery({
    queryKey: keys.templates(),
    queryFn: () => call(api.GET("/api/v1/templates")),
  });
}

export function useCreateNotification() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({ input, idempotencyKey }: { input: NotificationInput; idempotencyKey?: string }) => {
      let replayed = false;
      const notification = await call(
        api
          .POST("/api/v1/notifications", {
            body: input,
            params: { header: idempotencyKey ? { "Idempotency-Key": idempotencyKey } : {} },
          })
          .then((res) => {
            replayed = res.response.headers.get("Idempotent-Replayed") === "true";
            return res;
          }),
      );
      return { notification, replayed };
    },
    onSuccess: ({ notification }) => {
      client.setQueryData(keys.notification(notification.id), notification);
      void client.invalidateQueries({ queryKey: keys.lists() });
    },
  });
}

export function useCreateBatch() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ items, idempotencyKey }: { items: NotificationInput[]; idempotencyKey?: string }) =>
      call(
        api.POST("/api/v1/batches", {
          body: { notifications: items },
          params: { header: idempotencyKey ? { "Idempotency-Key": idempotencyKey } : {} },
        }),
      ),
    onSuccess: (batch) => {
      client.setQueryData(keys.batch(batch.id), batch);
      void client.invalidateQueries({ queryKey: keys.lists() });
    },
  });
}

export function useCancelNotification() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      call(api.POST("/api/v1/notifications/{id}/cancel", { params: { path: { id } } })),
    onSuccess: (n) => {
      client.setQueryData(keys.notification(n.id), n);
      void client.invalidateQueries({ queryKey: keys.lists() });
    },
  });
}

export function useRetryNotification() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      call(api.POST("/api/v1/notifications/{id}/retry", { params: { path: { id } } })),
    onSuccess: (n) => {
      client.setQueryData(keys.notification(n.id), n);
      void client.invalidateQueries({ queryKey: keys.attempts(n.id) });
      void client.invalidateQueries({ queryKey: keys.lists() });
    },
  });
}

export function useCancelBatch() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => call(api.POST("/api/v1/batches/{id}/cancel", { params: { path: { id } } })),
    onSuccess: (res) => {
      client.setQueryData(keys.batch(res.batch.id), res.batch);
      void client.invalidateQueries({ queryKey: keys.lists() });
    },
  });
}

export function useSaveTemplate() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id?: string; input: TemplateInput }) =>
      id
        ? call(api.PUT("/api/v1/templates/{id}", { params: { path: { id } }, body: input }))
        : call(api.POST("/api/v1/templates", { body: input })),
    onSuccess: () => client.invalidateQueries({ queryKey: keys.templates() }),
  });
}

export function useDeleteTemplate() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => call(api.DELETE("/api/v1/templates/{id}", { params: { path: { id } } })),
    onSuccess: () => client.invalidateQueries({ queryKey: keys.templates() }),
  });
}

export function usePreviewTemplate() {
  return useMutation({
    mutationFn: ({ id, variables }: { id: string; variables: Record<string, string> }) =>
      call(api.POST("/api/v1/templates/{id}/preview", { params: { path: { id } }, body: { variables } })),
  });
}
