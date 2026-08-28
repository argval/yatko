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
          <div className="space-y-2 max-w-md mx-auto tracking-normal">
            <p className="text-base sm:text-lg leading-relaxed">
              The download button GitHub forgot to add.
            </p>
            <p className="text-sm sm:text-base text-muted leading-relaxed">
              Yatko turns any public GitHub repository into a clean download link that picks the
              right release for each visitor&apos;s OS and architecture.
            </p>
          </div>
        </div>

        <div className="space-y-4">
          <HomeSearchSection />
          <HomeExamples />
        </div>

        <a
          href="https://www.producthunt.com/products/yatko?embed=true&utm_source=badge-featured&utm_medium=badge&utm_campaign=badge-yatko"
          target="_blank"
          rel="noopener noreferrer"
          className="mx-auto block w-fit"
        >
          <img
            src="https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=1227238&theme=light&t=1787658051325"
            alt="Yatko – The download button GitHub forgot to add on Product Hunt"
            width={250}
            height={54}
            className="dark:hidden"
          />
          <img
            src="https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=1227238&theme=dark&t=1787658051325"
            alt=""
            width={250}
            height={54}
            className="hidden dark:block"
          />
        </a>

        <HomeHowItWorks />
      </div>
    </main>
  );
}
