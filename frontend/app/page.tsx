import { HomeSearchSection } from "./home-search-section";
import { HomeExamples } from "./home-examples";
import { HomeHowItWorks } from "./home-how-it-works";
import { SITE_DOMAIN } from "@/lib/site";

export default function Home() {
  return (
    <main className="flex-1 flex flex-col items-center justify-center px-4 py-20 min-h-[100dvh]">
      <div className="w-full max-w-xl space-y-14 text-center">
        <div className="space-y-5">
          <div className="space-y-2">
            <h1 className="text-5xl sm:text-7xl font-semibold tracking-tighter leading-[1.05]">
              Yatko
            </h1>
            <p className="text-sm sm:text-base text-muted font-mono tracking-tight">
              {SITE_DOMAIN}
            </p>
          </div>
          <p className="text-base sm:text-lg text-muted leading-relaxed max-w-md mx-auto tracking-normal">
            Clean download links for any public GitHub release so that you don&apos;t have to be
            called a{" "}
            <a
              target="_blank"
              rel="noopener noreferrer"
              href="https://www.reddit.com/r/github/s/7YaS7nTVup"
              className="font-medium text-fg-brand hover:underline"
            >
              &quot;Smelly Nerd&quot;
            </a>{" "}
            anymore. Swap{" "}
            <span className="text-foreground/80 font-medium">github.com</span> for{" "}
            <span className="text-foreground/80 font-medium">{SITE_DOMAIN}</span> — we pick the
            right binary for each visitor&apos;s OS and architecture.
          </p>
        </div>

        <div className="space-y-4">
          <HomeSearchSection />
          <HomeExamples />
        </div>

        <HomeHowItWorks />
      </div>
    </main>
  );
}
