/** Browser observations travel with both the recommendation and its pinned download. */
export type VisitorContext = {
  platformSource: "browser" | "client-hints" | "manual";
  archSource: "browser" | "client-hints" | "manual";
  clientHints?: {
    platform?: string;
    architecture?: string;
    bitness?: string;
    platformVersion?: string;
  };
};
