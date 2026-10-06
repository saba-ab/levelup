import Navbar from "@/components/Navbar";
import HeroSection from "@/components/HeroSection";
import FeaturesSection from "@/components/FeaturesSection";
import HowItWorksSection from "@/components/HowItWorksSection";
import BenefitsSection from "@/components/BenefitsSection";
import UseCasesSection from "@/components/UseCasesSection";
import APIPreviewSection from "@/components/APIPreviewSection";
import SDKSection from "@/components/SDKSection";
import TechHighlightsSection from "@/components/TechHighlightsSection";
import PricingSection from "@/components/PricingSection";
import CTASection from "@/components/CTASection";
import Footer from "@/components/Footer";

export default function Home() {
  return (
    <div className="min-h-screen bg-background">
      <Navbar />
      <main>
        <HeroSection />
        <FeaturesSection />
        <HowItWorksSection />
        <BenefitsSection />
        <UseCasesSection />
        <APIPreviewSection />
        <SDKSection />
        <TechHighlightsSection />
        <PricingSection />
        <CTASection />
      </main>
      <Footer />
    </div>
  );
}
