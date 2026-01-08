# LevelUpOS Landing Page

A modern, high-performance landing page built with Next.js 14, React, TypeScript, and Tailwind CSS.

## 🚀 Tech Stack

- **Framework**: [Next.js 14](https://nextjs.org/) with App Router
- **Language**: [TypeScript](https://www.typescriptlang.org/)
- **Styling**: [Tailwind CSS](https://tailwindcss.com/)
- **UI Components**: [shadcn/ui](https://ui.shadcn.com/)
- **Icons**: [Lucide React](https://lucide.dev/)
- **Animations**: Custom CSS animations with Tailwind
- **State Management**: React Query for server state

## 📦 Getting Started

### Prerequisites

- Node.js 18.x or higher
- npm, yarn, or pnpm

### Installation

1. Install dependencies:

```bash
npm install
# or
yarn install
# or
pnpm install
```

2. Run the development server:

```bash
npm run dev
# or
yarn dev
# or
pnpm dev
```

3. Open [http://localhost:3000](http://localhost:3000) in your browser.

## 🏗️ Project Structure

```
levelupos-landing/
├── src/
│   ├── app/                    # Next.js App Router
│   │   ├── layout.tsx         # Root layout
│   │   ├── page.tsx           # Home page
│   │   ├── not-found.tsx      # 404 page
│   │   └── globals.css        # Global styles
│   ├── components/            # React components
│   │   ├── ui/               # shadcn/ui components
│   │   ├── providers/        # Context providers
│   │   ├── Navbar.tsx
│   │   ├── HeroSection.tsx
│   │   ├── FeaturesSection.tsx
│   │   ├── HowItWorksSection.tsx
│   │   ├── BenefitsSection.tsx
│   │   ├── UseCasesSection.tsx
│   │   ├── APIPreviewSection.tsx
│   │   ├── TechHighlightsSection.tsx
│   │   ├── PricingSection.tsx
│   │   ├── CTASection.tsx
│   │   └── Footer.tsx
│   ├── hooks/                # Custom React hooks
│   └── lib/                  # Utility functions
├── public/                   # Static assets
├── next.config.js           # Next.js configuration
├── tailwind.config.ts       # Tailwind CSS configuration
├── tsconfig.json            # TypeScript configuration
└── package.json             # Dependencies

```

## 🎨 Features

- ⚡ **Next.js 14** with App Router for optimal performance
- 🎯 **Type-safe** with TypeScript
- 🎨 **Modern UI** with shadcn/ui components
- 📱 **Fully Responsive** design
- ♿ **Accessible** components
- 🚀 **Optimized** for production
- 🎭 **Custom Animations** and gradients
- 🌙 **Dark Theme** optimized

## 🛠️ Available Scripts

- `npm run dev` - Start development server
- `npm run build` - Build for production
- `npm run start` - Start production server
- `npm run lint` - Run ESLint

## 🎨 Customization

### Colors

The color scheme is defined in `src/app/globals.css` using CSS variables. Key colors include:

- **Cyan** (`--cyan`): Primary accent color
- **Purple** (`--purple`): Secondary accent color
- **Gold** (`--gold`): Highlight color

### Fonts

The project uses:
- **Inter** for body text
- **Space Grotesk** for headings

Fonts are loaded via Google Fonts in the root layout.

## 📝 License

This project is private and proprietary.

## 🤝 Contributing

This is a private project. Please contact the team for contribution guidelines.
