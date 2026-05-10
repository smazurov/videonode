import React from "react";
import {
  Dialog,
  DialogPanel,
  DialogTitle,
  Transition,
  TransitionChild,
} from "@headlessui/react";
import { XMarkIcon } from "@heroicons/react/24/outline";
import { IconButton } from "./IconButton";
import { cn } from "../utils";

type MaxWidth = "sm" | "md" | "lg" | "xl" | "2xl" | "3xl" | "4xl" | "5xl" | "6xl";

const MAX_WIDTH_CLASSES: Record<MaxWidth, string> = {
  sm: "max-w-sm",
  md: "max-w-md",
  lg: "max-w-lg",
  xl: "max-w-xl",
  "2xl": "max-w-2xl",
  "3xl": "max-w-3xl",
  "4xl": "max-w-4xl",
  "5xl": "max-w-5xl",
  "6xl": "max-w-6xl",
};

interface BottomSheetProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly title: React.ReactNode;
  readonly maxWidth?: MaxWidth;
  readonly headerRight?: React.ReactNode;
  readonly headerExtra?: React.ReactNode;
  readonly padding?: boolean;
  readonly maxHeight?: string;
  readonly panelClassName?: string;
  readonly contentClassName?: string;
  readonly children: React.ReactNode;
}

// Owns the Dialog/Transition scaffold + backdrop + panel + header (title, optional
// right slot, close IconButton). Consumers just supply content via children.
export function BottomSheet({
  open,
  onClose,
  title,
  maxWidth = "4xl",
  headerRight,
  headerExtra,
  padding = true,
  maxHeight,
  panelClassName,
  contentClassName,
  children,
}: BottomSheetProps) {
  return (
    <Transition show={open}>
      <Dialog onClose={onClose} className="relative z-50">
        <TransitionChild
          enter="ease-out duration-300"
          enterFrom="opacity-0"
          enterTo="opacity-100"
          leave="ease-in duration-200"
          leaveFrom="opacity-100"
          leaveTo="opacity-0"
        >
          <div className="fixed inset-0 bg-surface-overlay" aria-hidden="true" />
        </TransitionChild>

        <div className="fixed inset-x-0 bottom-0 flex items-end justify-center">
          <TransitionChild
            enter="ease-out duration-300"
            enterFrom="opacity-0 translate-y-full"
            enterTo="opacity-100 translate-y-0"
            leave="ease-in duration-200"
            leaveFrom="opacity-100 translate-y-0"
            leaveTo="opacity-0 translate-y-full"
          >
            <DialogPanel
              className={cn(
                "w-full bg-surface-raised rounded-t-2xl shadow-xl flex flex-col",
                MAX_WIDTH_CLASSES[maxWidth],
                maxHeight,
                panelClassName,
              )}
            >
              <div className={cn("flex items-center justify-between shrink-0", padding ? "px-6 pt-6 pb-2" : "px-4 pt-4 pb-2")}>
                <div className="flex items-center gap-3 min-w-0">
                  <DialogTitle className="text-lg font-semibold text-fg truncate">{title}</DialogTitle>
                  {headerExtra}
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  {headerRight}
                  <IconButton icon={XMarkIcon} label="Close" onClick={onClose} theme="blank" />
                </div>
              </div>
              <div
                className={cn(
                  "flex-1 min-h-0 flex flex-col",
                  padding ? "px-6 pb-6" : "",
                  contentClassName,
                )}
              >
                {children}
              </div>
            </DialogPanel>
          </TransitionChild>
        </div>
      </Dialog>
    </Transition>
  );
}
