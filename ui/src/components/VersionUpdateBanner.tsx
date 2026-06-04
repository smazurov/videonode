import { ArrowPathIcon } from "@heroicons/react/24/outline";

import { useVersionUpdatePending } from "../lib/versionWatch";
import { Button } from "./Button";

// VersionUpdateBanner appears only when a new build was detected while the user
// was busy (silent reload is skipped mid-edit — see versionWatch). It offers an
// explicit reload so the deferred update never gets stuck behind active focus.
export function VersionUpdateBanner() {
  const pending = useVersionUpdatePending();
  if (!pending) return null;

  return (
    <div className="fixed inset-x-0 bottom-0 z-50 flex items-center justify-center gap-x-3 border-t border-border bg-surface-raised px-4 py-2 text-sm text-fg shadow-lg">
      <span>A new version is available.</span>
      <Button
        size="SM"
        theme="primary"
        text="Reload"
        LeadingIcon={ArrowPathIcon}
        onClick={() => window.location.reload()}
      />
    </div>
  );
}
