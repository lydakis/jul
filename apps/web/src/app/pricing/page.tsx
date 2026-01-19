import Link from "next/link";
import { Header, Footer } from "@/components/layout";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

const plans = [
  {
    name: "Personal",
    price: "Free",
    period: "forever",
    description: "Perfect for individual developers and side projects",
    features: [
      "Unlimited private repositories",
      "Sync-by-default for all repos",
      "Change-centric history",
      "Basic CI (build, test)",
      "30-day history retention",
      "1 workspace per repo",
    ],
    cta: "Get Started",
    href: "/signup",
    highlighted: false,
  },
  {
    name: "Pro",
    price: "$12",
    period: "per month",
    description: "For power users who need more from their Git hosting",
    features: [
      "Everything in Personal",
      "Unlimited workspaces",
      "90-day history retention",
      "Advanced CI (coverage, lint)",
      "AI-powered suggestions",
      "Priority support",
      "Custom domains",
    ],
    cta: "Start Free Trial",
    href: "/signup?plan=pro",
    highlighted: true,
    badge: "Most Popular",
  },
  {
    name: "Team",
    price: "$29",
    period: "per user/month",
    description: "For teams building together with AI assistance",
    features: [
      "Everything in Pro",
      "Unlimited team members",
      "1-year history retention",
      "Team workspaces",
      "Code review workflows",
      "SAML SSO",
      "Audit logs",
      "Dedicated support",
    ],
    cta: "Contact Sales",
    href: "/contact?plan=team",
    highlighted: false,
    badge: "Coming Soon",
  },
];

const faqs = [
  {
    question: "What counts as a repository?",
    answer:
      "A repository is any Git repository you host on Jul. There's no limit on repository size or number of commits.",
  },
  {
    question: "What is sync-by-default?",
    answer:
      "Sync-by-default means every commit you make is automatically backed up to Jul's servers. You never lose work, even if you forget to push.",
  },
  {
    question: "Can I use Jul with existing Git tools?",
    answer:
      "Yes! Jul is a standard Git remote. You can use any Git client, IDE, or tool that supports Git. The Jul CLI adds extra features but isn't required.",
  },
  {
    question: "What are AI-powered suggestions?",
    answer:
      "When your tests fail or code needs formatting, Jul's AI can suggest fixes. You review and apply them with a single command.",
  },
  {
    question: "Is my code secure?",
    answer:
      "Yes. All data is encrypted in transit and at rest. Private repositories are only accessible to you. We never train AI models on your code without explicit permission.",
  },
];

export default function PricingPage() {
  return (
    <div className="min-h-screen flex flex-col">
      <Header />
      <main className="flex-1 pt-24 pb-16">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          {/* Header */}
          <div className="text-center">
            <h1 className="font-display text-4xl font-semibold tracking-tight sm:text-5xl">
              Simple, transparent pricing
            </h1>
            <p className="mt-4 text-lg text-muted-foreground">
              Start free, upgrade when you need more
            </p>
          </div>

          {/* Pricing cards */}
          <div className="mt-16 grid gap-8 lg:grid-cols-3">
            {plans.map((plan) => (
              <div
                key={plan.name}
                className={`relative rounded-2xl border ${
                  plan.highlighted
                    ? "border-primary bg-card shadow-xl shadow-primary/10"
                    : "border-border bg-card/50"
                } p-8`}
              >
                {plan.badge && (
                  <Badge
                    className={`absolute -top-3 left-1/2 -translate-x-1/2 ${
                      plan.badge === "Coming Soon" ? "bg-muted text-muted-foreground" : ""
                    }`}
                  >
                    {plan.badge}
                  </Badge>
                )}

                <div className="text-center">
                  <h2 className="font-display text-xl font-semibold">{plan.name}</h2>
                  <div className="mt-4">
                    <span className="text-4xl font-bold">{plan.price}</span>
                    {plan.period !== "forever" && (
                      <span className="text-muted-foreground ml-1">/{plan.period}</span>
                    )}
                  </div>
                  <p className="mt-2 text-sm text-muted-foreground">{plan.description}</p>
                </div>

                <ul className="mt-8 space-y-3">
                  {plan.features.map((feature) => (
                    <li key={feature} className="flex items-start gap-3">
                      <span className="text-primary mt-0.5">&#10003;</span>
                      <span className="text-sm">{feature}</span>
                    </li>
                  ))}
                </ul>

                <Button
                  className="w-full mt-8"
                  variant={plan.highlighted ? "default" : "outline"}
                  asChild
                >
                  <Link href={plan.href}>{plan.cta}</Link>
                </Button>
              </div>
            ))}
          </div>

          {/* FAQ */}
          <div className="mt-24">
            <h2 className="font-display text-2xl font-semibold text-center mb-12">
              Frequently Asked Questions
            </h2>
            <div className="grid gap-6 lg:grid-cols-2">
              {faqs.map((faq) => (
                <div
                  key={faq.question}
                  className="rounded-lg border border-border/50 bg-card/30 p-6"
                >
                  <h3 className="font-medium">{faq.question}</h3>
                  <p className="mt-2 text-sm text-muted-foreground">{faq.answer}</p>
                </div>
              ))}
            </div>
          </div>

          {/* CTA */}
          <div className="mt-24 text-center">
            <h2 className="font-display text-2xl font-semibold">
              Ready to never lose work again?
            </h2>
            <p className="mt-2 text-muted-foreground">
              Start for free, no credit card required
            </p>
            <Button size="lg" className="mt-6" asChild>
              <Link href="/signup">Get Started for Free</Link>
            </Button>
          </div>
        </div>
      </main>
      <Footer />
    </div>
  );
}
