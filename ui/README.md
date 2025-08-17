# VideoNode UI

The React TypeScript frontend for VideoNode - a video streaming and device management platform.

## Development

### Prerequisites
- Node.js 22.15.0 (or compatible version)
- npm

### Setup
```bash
npm install
```

### Development Server
```bash
npm run dev
```

The development server will start on `http://localhost:3000` with proxy configuration to forward API calls to the VideoNode backend at `http://localhost:8090`.

### Build
```bash
npm run build
```

### Linting
```bash
npm run lint
npm run lint:fix
```

## Project Structure

```
src/
├── components/     # Reusable UI components
├── hooks/         # Custom React hooks
├── assets/        # Static assets (images, icons, etc.)
├── routes/        # Route components and pages
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
- Dark/Light mode support
- Responsive design with Tailwind CSS
- Custom Circular font family
- ESLint and Prettier configured
- Path aliasing for clean imports (@components/*, @routes/*, etc.)
- API proxy to VideoNode backend
