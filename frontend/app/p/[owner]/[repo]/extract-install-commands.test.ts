import { describe, expect, test } from "bun:test";
import { extractInstallCommands } from "./extract-install-commands";

describe("extractInstallCommands", () => {
  test("parses backtick fences", () => {
    const readme = ["## Install", "```", "brew install foo", "```"].join("\n");
    expect(extractInstallCommands(readme)).toEqual([
      { command: "brew install foo", platform: "macos" },
    ]);
  });

  test("parses tilde fences used by some READMEs", () => {
    const readme = [
      "**macOS**",
      "~~~ shell",
      "brew install ncurses automake autoconf gcc",
      "~~~",
      "",
      "**Debian/Ubuntu**",
      "~~~ shell",
      "sudo apt install libncursesw5-dev build-essential",
      "~~~",
    ].join("\n");
    expect(extractInstallCommands(readme)).toEqual([
      { command: "brew install ncurses automake autoconf gcc", platform: "macos" },
      { command: "sudo apt install libncursesw5-dev build-essential", platform: "linux" },
    ]);
  });

  test("keeps sudo/doas and matches dnf/zypper/pacman", () => {
    const readme = [
      "```",
      "sudo dnf install ncurses-devel",
      "sudo zypper install ncurses-devel",
      "sudo pacman -S --needed base-devel ncurses",
      "doas apt-get install foo",
      "```",
    ].join("\n");
    expect(extractInstallCommands(readme)).toEqual([
      { command: "sudo dnf install ncurses-devel", platform: "linux" },
      { command: "sudo zypper install ncurses-devel", platform: "linux" },
      { command: "sudo pacman -S --needed base-devel ncurses", platform: "linux" },
      { command: "doas apt-get install foo", platform: "linux" },
    ]);
  });

  test("ignores install lines outside fences", () => {
    expect(extractInstallCommands("Just run brew install foo")).toEqual([]);
  });
});
