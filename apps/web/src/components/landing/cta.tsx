import { Button } from "@/components/ui/button";
import Link from "next/link";

export function CTA() {
  return (
    <section className="py-20 sm:py-32">
      <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
        <div className="relative overflow-hidden rounded-3xl bg-gradient-to-br from-primary/20 via-accent/10 to-background border border-border/50 px-6 py-16 sm:px-12 sm:py-24">
          {/* Background decoration */}
          <div className="absolute inset-0 -z-10">
            <div className="absolute top-0 right-0 w-96 h-96 bg-primary/10 rounded-full blur-3xl" />
            <div className="absolute bottom-0 left-0 w-96 h-96 bg-accent/10 rounded-full blur-3xl" />
          </div>

          <div className="text-center">
            <h2 className="font-display text-3xl font-semibold tracking-tight sm:text-4xl lg:text-5xl">
              Ready to never lose work again?
            </h2>
            <p className="mx-auto mt-6 max-w-xl text-lg text-muted-foreground">
              Join the beta and experience Git hosting built for the age of agents.
              Free for personal use.
            </p>
            <div className="mt-10 flex flex-col items-center justify-center gap-4 sm:flex-row">
              <Button size="lg" asChild className="min-w-[200px]">
                <Link href="/signup">
                  Get started for free
                </Link>
              </Button>
              <Button size="lg" variant="outline" asChild className="min-w-[200px]">
                <Link href="https://github.com/jul-sh/jul" target="_blank" rel="noopener noreferrer">
                  Star on GitHub
                </Link>
              </Button>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
