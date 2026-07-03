import { createFileRoute } from "@tanstack/react-router"

import { Cta } from "@/features/landing/components/cta"
import { FeatureGrid } from "@/features/landing/components/feature-grid"
import { Hero } from "@/features/landing/components/hero"
import { Install } from "@/features/landing/components/install"
import { LandingFooter } from "@/features/landing/components/landing-footer"
import { LandingNav } from "@/features/landing/components/landing-nav"
import { Metrics } from "@/features/landing/components/metrics"
import { ProductShowcase } from "@/features/landing/components/product-showcase"

export const Route = createFileRoute("/")({ component: LandingPage })

function LandingPage() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <LandingNav />
      <Hero />
      <ProductShowcase />
      <FeatureGrid />
      <Metrics />
      <Install />
      <Cta />
      <LandingFooter />
    </div>
  )
}
