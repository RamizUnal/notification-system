import { render, screen } from "@testing-library/react";

import { LevelMeter } from "./LevelMeter";

describe("LevelMeter", () => {
  it("exposes its reading to assistive technology", () => {
    render(<LevelMeter label="SMS" value={97} peak={100} ceiling={100} />);
    const meter = screen.getByRole("meter", { name: "SMS throughput" });
    expect(meter).toHaveAttribute("aria-valuenow", "97");
    expect(meter).toHaveAttribute("aria-valuemax", "100");
    expect(meter).toHaveAttribute("aria-valuetext", "97 per second of a 100 per second ceiling");
  });

  it("lights segments in proportion and the top ones only at the ceiling", () => {
    const { container, rerender } = render(<LevelMeter label="SMS" value={50} peak={50} ceiling={100} />);
    expect(container.querySelectorAll("[data-lit]")).toHaveLength(10);
    expect(container.querySelectorAll("[data-lit][data-hot]")).toHaveLength(0);

    rerender(<LevelMeter label="SMS" value={100} peak={100} ceiling={100} />);
    expect(container.querySelectorAll("[data-lit]")).toHaveLength(20);
    expect(container.querySelectorAll("[data-lit][data-hot]")).toHaveLength(2);
  });

  it("holds the recent peak above the current level", () => {
    const { container } = render(<LevelMeter label="SMS" value={20} peak={80} ceiling={100} />);
    expect(container.querySelectorAll("[data-peak]")).toHaveLength(1);
  });
});
