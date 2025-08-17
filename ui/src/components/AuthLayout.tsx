import { useSearchParams } from "react-router-dom";
import SimpleNavbar from "./SimpleNavbar";
import Container from "./Container";
import Fieldset from "./Fieldset";
import GridBackground from "./GridBackground";

interface AuthLayoutProps {
  title: string;
  description: string;
  children: React.ReactNode;
  showNavbar?: boolean;
}

export default function AuthLayout({
  title,
  description,
  children,
  showNavbar = true,
}: Readonly<AuthLayoutProps>) {
  const [sq] = useSearchParams();

  // Get returnTo parameter for potential future use
  const returnTo = sq.get("returnTo");
  console.log('Return to:', returnTo); // Suppress unused variable warning

  return (
    <>
      <GridBackground />

      <div className="grid min-h-screen" style={{ gridTemplateRows: showNavbar ? "auto 1fr" : "1fr" }}>
        {showNavbar && (
          <SimpleNavbar
            logoHref="/"
            logoText="VideoNode"
            actionElement={null}
          />
        )}
        <Container>
          <div className="isolate flex h-full w-full items-center justify-center">
            <div className="-mt-16 max-w-2xl space-y-8">
              <div className="space-y-2 text-center">
                <h1 className="text-4xl font-semibold text-black dark:text-white">
                  {title}
                </h1>
                <p className="text-slate-600 dark:text-slate-400">{description}</p>
              </div>

              <Fieldset className="space-y-12 border-slate-800/30 dark:border-slate-300/20">
                <div className="mx-auto max-w-sm space-y-4">
                  {children}
                </div>
              </Fieldset>
            </div>
          </div>
        </Container>
      </div>
    </>
  );
}