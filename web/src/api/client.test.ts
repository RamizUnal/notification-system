import { ApiError, call } from "./client";

function response(status: number, body?: unknown) {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/problem+json" },
  });
}

describe("call", () => {
  it("returns data on success", async () => {
    await expect(call(Promise.resolve({ data: { ok: true }, response: response(200) }))).resolves.toEqual({ ok: true });
  });

  it("raises the API's problem details", async () => {
    const problem = {
      title: "Unprocessable Entity",
      status: 422,
      code: "validation_failed",
      request_id: "req-1",
      errors: [
        { field: "recipient", code: "invalid_format", message: "recipient must be an E.164 phone number" },
        { field: "recipient", code: "required", message: "a second message for the same field is ignored" },
      ],
    };

    const err = await call(Promise.resolve({ error: problem, response: response(422, problem) })).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ApiError);
    const apiErr = err as ApiError;
    expect(apiErr.status).toBe(422);
    expect(apiErr.problem.request_id).toBe("req-1");
    expect(apiErr.fieldErrors).toEqual({ recipient: "recipient must be an E.164 phone number" });
  });

  it("synthesizes a problem when the API cannot be reached", async () => {
    const err = await call(Promise.reject(new TypeError("Failed to fetch"))).catch((e: unknown) => e);
    expect((err as ApiError).problem.code).toBe("network_error");
    expect((err as ApiError).status).toBe(0);
  });

  it("explains responses without problem details", async () => {
    const err = await call(Promise.resolve({ error: "oops", response: response(502) })).catch((e: unknown) => e);
    expect((err as ApiError).problem.code).toBe("unexpected_response");
  });
});
