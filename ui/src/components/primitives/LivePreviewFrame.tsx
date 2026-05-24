import { ReactNode } from "react";
import { cn } from "../../utils";
import { Spinner } from "../Spinner";

interface LivePreviewFrameProps {
  src?: string | undefined;
  loading?: boolean;
  error?: string | null;
  overlay?: ReactNode;
  aspect?: "16/9" | "4/3" | "1/1";
  className?: string;
  alt?: string;
}

const aspectClass: Record<NonNullable<LivePreviewFrameProps["aspect"]>, string> = {
  "16/9": "aspect-video",
  "4/3": "aspect-[4/3]",
  "1/1": "aspect-square",
};

function PlaceholderContent({ loading, error }: Readonly<{ loading: boolean; error: string | null }>) {
  if (loading) return <Spinner />;
  if (error) return <span className="text-danger">{error}</span>;
  return <>No preview</>;
}

export function LivePreviewFrame({
  src,
  loading = false,
  error = null,
  overlay,
  aspect = "16/9",
  className,
  alt = "Live preview",
}: Readonly<LivePreviewFrameProps>) {
  return (
    <div
      className={cn(
        "relative w-full overflow-hidden rounded-md bg-black border border-border",
        aspectClass[aspect],
        className,
      )}
    >
      {src ? (
        <img src={src} alt={alt} className="absolute inset-0 w-full h-full object-contain" />
      ) : (
        <div className="absolute inset-0 flex items-center justify-center text-fg-subtle text-sm">
          <PlaceholderContent loading={loading} error={error} />
        </div>
      )}
      {loading && src && (
        <div className="absolute top-2 right-2">
          <Spinner />
        </div>
      )}
      {overlay && <div className="absolute inset-0 pointer-events-none">{overlay}</div>}
    </div>
  );
}
