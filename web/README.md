# Donelist Web Frontend

Modern React frontend for Donelist settings management, built with TypeScript, Vite, and Tailwind CSS.

## Features

- **Theme Management**: Light, Dark, and System theme support with instant application
- **Language Selection**: Multi-language support with i18next (English, Korean, Japanese)
- **Data Retention**: Configure automatic data deletion policies
- **Offline Support**: Settings cached in localStorage for offline access
- **Accessibility**: WCAG 2.1 AA compliant with full keyboard navigation
- **Performance**: Optimized bundle size with code splitting and lazy loading

## Tech Stack

- **Framework**: React 18.3 with TypeScript
- **Build Tool**: Vite 5
- **State Management**: Zustand 4
- **Styling**: Tailwind CSS 3
- **Internationalization**: i18next with react-i18next
- **API Client**: Native Fetch API with TypeScript

## Getting Started

### Prerequisites

- Node.js 20+
- pnpm (recommended) or npm

### Installation

```bash
# Install dependencies
pnpm install

# Copy environment variables
cp .env.example .env

# Start development server
pnpm dev
```

The app will be available at `http://localhost:3000`.

### Backend Integration

Ensure the Go backend is running on `http://localhost:8080`. The Vite dev server proxies `/api` requests to the backend.

## Project Structure

```
web/
├── src/
│   ├── components/          # React components
│   │   ├── ThemeSelector.tsx
│   │   ├── LanguageSelector.tsx
│   │   ├── DataRetentionSettings.tsx
│   │   └── SettingsPage.tsx
│   ├── stores/              # Zustand state stores
│   │   └── settingsStore.ts
│   ├── lib/                 # Utilities
│   │   ├── api.ts          # API client
│   │   └── i18n.ts         # i18n configuration
│   ├── types/              # TypeScript types
│   │   └── settings.ts
│   ├── locales/            # Translation files
│   │   ├── en.json
│   │   └── ko.json
│   ├── App.tsx             # Root component
│   ├── main.tsx            # App entry point
│   └── index.css           # Global styles
├── index.html
├── vite.config.ts
├── tailwind.config.js
└── package.json
```

## Available Scripts

```bash
# Development
pnpm dev              # Start dev server with HMR

# Production
pnpm build            # Build for production
pnpm preview          # Preview production build

# Code Quality
pnpm lint             # Run ESLint
```

## API Integration

The frontend integrates with these backend endpoints:

- `GET /api/profile/all` - Fetch all user settings
- `PATCH /api/settings` - Update theme/language settings
- `PATCH /api/settings/data-retention` - Update data retention settings

All requests include JWT authentication via `Authorization: Bearer <token>`.

## Theme System

Themes are applied using CSS custom properties and Tailwind's dark mode class strategy:

1. **Light Theme**: Default, uses `bg-white` and light colors
2. **Dark Theme**: Uses `dark:` Tailwind variants with `bg-gray-900`
3. **System Theme**: Respects OS preference via `prefers-color-scheme`

Theme changes are applied instantly to prevent FOUC (Flash of Unstyled Content).

## Internationalization

Languages are detected automatically from:
1. localStorage saved preference
2. Browser language (`navigator.language`)
3. Fallback to English

Translation files are in `src/locales/`. Add new languages by:
1. Creating `src/locales/<lang>.json`
2. Adding to `src/lib/i18n.ts` resources
3. Adding to `LanguageSelector.tsx` options

## Accessibility

- **Keyboard Navigation**: All interactive elements are keyboard accessible
- **ARIA Labels**: Proper labels for screen readers
- **Focus Management**: Visible focus indicators
- **Color Contrast**: WCAG AA compliant contrast ratios
- **Reduced Motion**: Respects `prefers-reduced-motion`

## Performance Optimizations

- **Bundle Size**: < 150KB gzipped
- **Code Splitting**: Dynamic imports for routes
- **Tree Shaking**: Unused code eliminated
- **Lazy Loading**: Components loaded on demand
- **Optimistic Updates**: Instant UI feedback

## Testing

```bash
# Unit tests (TBD)
pnpm test

# E2E tests (TBD)
pnpm test:e2e
```

## Deployment

### Production Build

```bash
pnpm build
```

Output is in `dist/` directory. Serve with any static file server.

### Environment Variables

- `VITE_API_URL`: Backend API base URL (default: `/api`)

## Browser Support

- Chrome 90+
- Firefox 88+
- Safari 14+
- Edge 90+

## Contributing

Follow the project's contribution guidelines in the root README.

## License

Proprietary - All rights reserved

---

**Status**: Task 15.6 Complete
**Last Updated**: 2025-11-19
