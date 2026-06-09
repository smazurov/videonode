# VideoNode UI

The React TypeScript frontend for VideoNode - a video streaming and device management platform.

## Development

### Prerequisites
- Node.js 22.15.0 (or compatible version)
- pnpm

### Setup
```bash
pnpm install
```

### Development Server
```bash
pnpm dev
```

The development server starts on `http://localhost:5173` (override with `VITE_DEV_PORT`) with proxy configuration to forward API calls to the VideoNode backend at `http://localhost:8090`.

### Build
```bash
pnpm build
```

### Linting
```bash
pnpm lint
pnpm lint:fix
```

## Project Structure

```
src/
├── components/     # Reusable UI components
├── design/        # Design system: tokens + primitives
├── hooks/         # Custom React hooks
├── lib/           # Shared library code
├── routes/        # Route components and pages
├── router.tsx     # Route configuration
├── main.tsx       # Application entry point
├── root.tsx       # Root layout component
├── utils.ts       # Utility functions
└── index.css      # Global styles with Tailwind CSS
```

## Technology Stack

- **React 19** with TypeScript
- **Vite** for build tooling
- **React Router** for client-side routing
- **Tailwind CSS** for styling
- **Headless UI** for accessible components
- **Heroicons** for icons
- **React Hot Toast** for notifications

## Features

- Modern React with TypeScript
- Dark mode infrastructure (CSS `.dark` selector; no in-app toggle)
- Responsive design with Tailwind CSS
- Custom Circular font family
- ESLint and Prettier configured
- Path aliases configured (`@components/*`, `@routes/*`, etc.; not currently used in source)
- API proxy to VideoNode backend
