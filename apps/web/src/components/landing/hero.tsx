import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import Link from "next/link";

export function Hero() {
  return (
    <section className="relative overflow-hidden pt-32 pb-20 sm:pt-40 sm:pb-32">
      {/* Background gradient */}
      <div className="absolute inset-0 -z-10">
        <div className="absolute top-0 left-1/2 -translate-x-1/2 w-[800px] h-[600px] bg-gradient-to-b from-primary/20 via-accent/10 to-transparent blur-3xl opacity-50" />
      </div>

      {/* Korean character decoration - subtle background element */}
      <div className="absolute top-32 right-[10%] -z-10 select-none pointer-events-none hidden lg:block">
        <span className="text-[12rem] font-light text-muted-foreground/[0.03] leading-none">
          줄
        </span>
      </div>

      <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
        <div className="text-center">
          {/* Badge */}
          <Badge variant="secondary" className="mb-6 px-4 py-1.5 text-sm">
            <span className="mr-2">&#10024;</span>
            Now in Beta
          </Badge>

          {/* Headline with Korean subtitle */}
          <h1 className="font-display text-4xl font-semibold tracking-tight sm:text-6xl lg:text-7xl">
            <span className="block">Git hosting for the</span>
            <span className="block mt-2 text-gradient-coral">
              age of agents
            </span>
          </h1>

          {/* Korean meaning */}
          <p className="mt-4 text-muted-foreground/60 text-sm tracking-widest uppercase">
            줄 <span className="mx-2">&#8226;</span> jul <span className="mx-2">&#8226;</span> line
          </p>

          {/* Subheadline */}
          <p className="mx-auto mt-8 max-w-2xl text-lg text-muted-foreground sm:text-xl">
            Sync-by-default. Change-centric history. Agent-native primitives.
            <br className="hidden sm:block" />
            <span className="text-foreground font-medium">Never lose work again.</span>
          </p>

          {/* CTA Buttons */}
          <div className="mt-10 flex flex-col items-center justify-center gap-4 sm:flex-row">
            <Button size="lg" asChild className="min-w-[180px]">
              <Link href="/signup">
                Start for free
                <span className="ml-2">&rarr;</span>
              </Link>
            </Button>
            <Button size="lg" variant="outline" asChild className="min-w-[180px]">
              <Link href="/docs">
                Read the docs
              </Link>
            </Button>
          </div>

          {/* Terminal preview */}
          <div className="mx-auto mt-16 max-w-3xl">
            <div className="rounded-xl border border-border bg-card p-1 shadow-2xl shadow-primary/5">
              <div className="flex items-center gap-2 px-4 py-3 border-b border-border">
                <div className="flex gap-1.5">
                  <div className="w-3 h-3 rounded-full bg-destructive/60" />
                  <div className="w-3 h-3 rounded-full bg-chart-4/60" />
                  <div className="w-3 h-3 rounded-full bg-chart-5/60" />
                </div>
                <span className="ml-2 text-xs text-muted-foreground font-mono">
                  terminal
                </span>
              </div>
              <div className="p-6 font-mono text-sm text-left">
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground">$</span>
                  <span className="text-foreground">jul status</span>
                </div>
                <div className="mt-4 space-y-2 text-muted-foreground">
                  <p>
                    <span className="text-foreground">Workspace:</span> george/macbook-main
                  </p>
                  <p>
                    <span className="text-foreground">Change-Id:</span>{" "}
                    <span className="text-accent">Iab4f3c2d</span> (rev 3)
                  </p>
                  <p className="mt-3">
                    <span className="text-foreground">Sync:</span>{" "}
                    <span className="text-chart-5">&#10003; up to date</span>
                  </p>
                  <p>
                    <span className="text-foreground">CI:</span>{" "}
                    <span className="text-chart-5">&#10003; tests passing</span>{" "}
                    <span className="text-muted-foreground">&#8226; 84.1% coverage</span>
                  </p>
                </div>
                <div className="mt-4 pt-4 border-t border-border/50">
                  <p className="text-chart-5">
                    &#10003; Ready to promote to main
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
