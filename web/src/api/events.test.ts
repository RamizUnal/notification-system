import { EventStream } from "./events";

/** A scriptable WebSocket stand-in. */
class FakeSocket {
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  onclose: ((e: { code: number }) => void) | null = null;
  onerror: (() => void) | null = null;
  close() {}
  open() {
    this.onopen?.();
  }
  send(type: string, data: Record<string, unknown>) {
    this.onmessage?.({ data: JSON.stringify({ type, data }) });
  }
  drop(code = 1006) {
    this.onclose?.({ code });
  }
}

/** Returns the i-th socket the stream opened, failing clearly if it never did. */
function nth(sockets: FakeSocket[], i: number): FakeSocket {
  const s = sockets[i];
  if (!s) throw new Error(`socket ${i} was never opened`);
  return s;
}

function setup() {
  const sockets: FakeSocket[] = [];
  const stream = new EventStream(
    () => "ws://test/api/v1/events",
    () => {
      const s = new FakeSocket();
      sockets.push(s);
      return s as unknown as WebSocket;
    },
  );
  stream.start();
  return { stream, sockets };
}

describe("EventStream", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("batches messages and keeps the log newest first", () => {
    const { stream, sockets } = setup();
    const socket = nth(sockets, 0);
    socket.open();
    expect(stream.getSnapshot().status).toBe("live");

    const notified = vi.fn();
    stream.subscribe(notified);
    socket.send("notification.updated", { notification_id: "a", channel: "sms", status: "sent", attempts: 1 });
    socket.send("notification.updated", { notification_id: "b", channel: "sms", status: "retrying", attempts: 1 });
    expect(notified).not.toHaveBeenCalled();

    vi.advanceTimersByTime(150);
    expect(notified).toHaveBeenCalledTimes(1);
    expect(stream.getSnapshot().log.map((e) => e.notificationId)).toEqual(["b", "a"]);
    stream.stop();
  });

  it("measures the live send rate and its peak per channel", () => {
    const { stream, sockets } = setup();
    const socket = nth(sockets, 0);
    socket.open();
    for (let i = 0; i < 40; i++) {
      socket.send("notification.updated", { notification_id: `n${i}`, channel: "email", status: "sent", attempts: 1 });
    }
    vi.advanceTimersByTime(150);
    expect(stream.rate("email")).toBe(40);
    expect(stream.rate("sms")).toBe(0);

    vi.advanceTimersByTime(1500);
    expect(stream.rate("email")).toBe(0);
    expect(stream.peak("email")).toBe(40);
    stream.stop();
  });

  it("reconnects with backoff and asks for a resync", () => {
    const { stream, sockets } = setup();
    const resync = vi.fn();
    stream.onResync(resync);
    nth(sockets, 0).open();
    expect(resync).not.toHaveBeenCalled();

    nth(sockets, 0).drop();
    expect(stream.getSnapshot().status).toBe("reconnecting");
    vi.advanceTimersByTime(1000);
    expect(sockets).toHaveLength(2);

    nth(sockets, 1).open();
    expect(resync).toHaveBeenCalledTimes(1);
    stream.stop();
  });

  it("reconnects at once when the server says the client fell behind", () => {
    const { stream, sockets } = setup();
    nth(sockets, 0).open();
    nth(sockets, 0).drop(4000);
    vi.advanceTimersByTime(0);
    expect(sockets).toHaveLength(2);
    stream.stop();
  });
});
