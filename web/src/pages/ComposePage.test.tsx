import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";

import { ComposePage } from "./ComposePage";

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/compose"]}>
        <ComposePage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function json(status: number, body: unknown, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": status >= 400 ? "application/problem+json" : "application/json", ...headers },
  });
}

describe("ComposePage", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("shows the server's validation errors next to the fields they concern", async () => {
    const fetchMock = vi.fn(async (request: Request) => {
      if (request.url.includes("/templates")) return json(200, { data: [] });
      return json(422, {
        type: "about:blank",
        title: "Unprocessable Entity",
        status: 422,
        code: "validation_failed",
        request_id: "req-42",
        errors: [{ field: "recipient", code: "invalid_format", message: "recipient must be an E.164 phone number such as +905551234567" }],
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage();

    await user.type(screen.getByLabelText("Phone number"), "05551234567");
    await user.type(screen.getByLabelText("Message"), "Your code is 482910");
    await user.click(screen.getByRole("button", { name: "Queue notification" }));

    expect(await screen.findByText(/must be an E\.164 phone number/)).toBeInTheDocument();
    expect(screen.getByLabelText("Phone number")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText("Some fields need attention")).toBeInTheDocument();
    expect(screen.getByText("req-42")).toBeInTheDocument();

    const sent = fetchMock.mock.calls.map(([r]) => r).find((r) => r.method === "POST");
    expect(await sent?.json()).toEqual({ channel: "sms", recipient: "05551234567", priority: "normal", content: "Your code is 482910" });
  });

  it("sends the idempotency key as a header and reports a replay", async () => {
    let seenKey: string | null = null;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.url.includes("/templates")) return json(200, { data: [] });
        if (request.method === "POST") {
          seenKey = request.headers.get("Idempotency-Key");
          return json(200, { id: "01a1fb2c-0000-7000-8000-000000000000", status: "queued" }, { "Idempotent-Replayed": "true" });
        }
        return json(200, { id: "01a1fb2c-0000-7000-8000-000000000000", status: "sent" });
      }),
    );
    const user = userEvent.setup();
    renderPage();

    await user.type(screen.getByLabelText("Phone number"), "+905551234567");
    await user.type(screen.getByLabelText("Message"), "hi");
    await user.type(screen.getByLabelText("Idempotency key"), "order-42");
    await user.click(screen.getByRole("button", { name: "Queue notification" }));

    expect(await screen.findByText("Already accepted earlier")).toBeInTheDocument();
    expect(seenKey).toBe("order-42");
  });

  it("counts SMS segments and encoding as you type", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => json(200, { data: [] })));
    const user = userEvent.setup();
    renderPage();

    await user.type(screen.getByLabelText("Message"), "Siparişiniz yolda");
    expect(screen.getByText(/17 \/ 1,600 · 1 segment · UCS-2/)).toBeInTheDocument();
  });
});
